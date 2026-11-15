package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// fileSink replica los logs en un fichero con rotación por tamaño. Sirve
// para conservar el historial (p. ej. por qué se perdió una sesión) aunque
// el contenedor se recree, ya que `docker logs` y el buffer del panel se
// pierden en ese caso.
type fileSink struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	backups  int
	f        *os.File
	size     int64
}

var (
	sinkMu sync.RWMutex
	sink   *fileSink
)

// EnableFileSink activa la escritura de logs en path. maxBytes <= 0 usa 5 MiB
// y backups <= 0 conserva 3 ficheros rotados (path.1 ... path.N).
func EnableFileSink(path string, maxBytes int64, backups int) error {
	if maxBytes <= 0 {
		maxBytes = 5 << 20
	}
	if backups <= 0 {
		backups = 3
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	s := &fileSink{path: path, maxBytes: maxBytes, backups: backups}
	if err := s.open(); err != nil {
		return err
	}
	sinkMu.Lock()
	old := sink
	sink = s
	sinkMu.Unlock()
	if old != nil {
		old.close()
	}
	return nil
}

// DisableFileSink cierra el fichero de log (usado en tests y al apagar).
func DisableFileSink() {
	sinkMu.Lock()
	old := sink
	sink = nil
	sinkMu.Unlock()
	if old != nil {
		old.close()
	}
}

// FileSinkPath devuelve la ruta del log persistente activo ("" si no hay).
func FileSinkPath() string {
	sinkMu.RLock()
	defer sinkMu.RUnlock()
	if sink == nil {
		return ""
	}
	return sink.path
}

func writeToSink(now time.Time, msg string) {
	sinkMu.RLock()
	s := sink
	sinkMu.RUnlock()
	if s == nil {
		return
	}
	s.write(fmt.Sprintf("%s %s\n", now.Format("2006-01-02 15:04:05"), msg))
}

func (s *fileSink) open() error {
	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	s.f = f
	s.size = info.Size()
	return nil
}

func (s *fileSink) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f != nil {
		_ = s.f.Close()
		s.f = nil
	}
}

func (s *fileSink) write(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return
	}
	if s.size+int64(len(line)) > s.maxBytes {
		s.rotateLocked()
		if s.f == nil {
			return
		}
	}
	n, err := s.f.WriteString(line)
	s.size += int64(n)
	if err != nil {
		// No usar Log aquí (recursión). Avisar por stderr una vez y desactivar.
		fmt.Fprintf(os.Stderr, "logger: error escribiendo en %s: %v (log persistente desactivado)\n", s.path, err)
		_ = s.f.Close()
		s.f = nil
	}
}

func (s *fileSink) rotateLocked() {
	_ = s.f.Close()
	s.f = nil
	for i := s.backups - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", s.path, i), fmt.Sprintf("%s.%d", s.path, i+1))
	}
	_ = os.Rename(s.path, s.path+".1")
	if err := s.open(); err != nil {
		fmt.Fprintf(os.Stderr, "logger: no se pudo reabrir %s tras rotar: %v\n", s.path, err)
	}
}
