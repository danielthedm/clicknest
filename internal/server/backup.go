package server

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// exportHandler streams a .tar.gz backup of the data directory.
// GET /api/v1/export
func (s *Server) exportHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Flush WAL to main files so copies are consistent.
	if err := s.events.Checkpoint(ctx); err != nil {
		log.Printf("WARN: export: checkpoint duckdb: %v", err)
	}
	if _, err := s.meta.DB().ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		log.Printf("WARN: export: checkpoint sqlite: %v", err)
	}

	filename := fmt.Sprintf("clicknest-backup-%s.tar.gz", time.Now().UTC().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	for _, name := range []string{"events.duckdb", "clicknest.db", ".encryption_key"} {
		path := filepath.Join(s.config.DataDir, name)
		f, err := os.Open(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			log.Printf("WARN: export: open %s: %v", name, err)
			continue
		}

		info, err := f.Stat()
		if err != nil {
			f.Close()
			continue
		}

		hdr := &tar.Header{
			Name:    name,
			Mode:    0600,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			f.Close()
			break
		}
		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			break
		}
		f.Close()
	}

	tw.Close()
	gz.Close()
}

// importHandler accepts a .tar.gz backup and restores it, then restarts the server.
// POST /api/v1/import  (multipart/form-data, field: "backup")
func (s *Server) importHandler(w http.ResponseWriter, r *http.Request) {
	// 10 GB max upload.
	r.Body = http.MaxBytesReader(w, r.Body, 10<<30)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, `{"error":"invalid multipart form"}`, http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("backup")
	if err != nil {
		http.Error(w, `{"error":"missing backup field"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	gr, err := gzip.NewReader(file)
	if err != nil {
		http.Error(w, `{"error":"invalid gzip archive"}`, http.StatusBadRequest)
		return
	}
	defer gr.Close()

	allowed := map[string]bool{
		"events.duckdb":    true,
		"clicknest.db":     true,
		".encryption_key":  true,
	}

	// Extract to temp files first, then swap after closing DBs.
	type tempEntry struct {
		finalPath string
		tempPath  string
	}
	var entries []tempEntry

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, `{"error":"corrupt archive"}`, http.StatusBadRequest)
			// Clean up temps.
			for _, e := range entries {
				os.Remove(e.tempPath)
			}
			return
		}

		name := filepath.Base(hdr.Name) // Sanitize: strip any directory traversal.
		if !allowed[name] {
			continue
		}

		tempPath := filepath.Join(s.config.DataDir, name+".import_tmp")
		f, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			http.Error(w, `{"error":"failed to write temp file"}`, http.StatusInternalServerError)
			for _, e := range entries {
				os.Remove(e.tempPath)
			}
			return
		}

		// Limit each individual file to 10 GB to prevent zip bombs.
		if _, err := io.Copy(f, io.LimitReader(tr, 10<<30)); err != nil {
			f.Close()
			os.Remove(tempPath)
			http.Error(w, `{"error":"failed to write temp file"}`, http.StatusInternalServerError)
			for _, e := range entries {
				os.Remove(e.tempPath)
			}
			return
		}
		f.Close()

		entries = append(entries, tempEntry{
			finalPath: filepath.Join(s.config.DataDir, name),
			tempPath:  tempPath,
		})
	}

	if len(entries) == 0 {
		http.Error(w, `{"error":"no recognizable files in archive"}`, http.StatusBadRequest)
		return
	}

	// Send success response before restarting.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Backup restored. Server is restarting — refresh in a few seconds.",
	})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Close DBs, swap files, exit.
	go func() {
		time.Sleep(200 * time.Millisecond)

		s.events.Close()
		s.meta.Close()

		for _, e := range entries {
			if err := os.Rename(e.tempPath, e.finalPath); err != nil {
				log.Printf("ERROR: import: rename %s: %v", e.finalPath, err)
			}
		}

		log.Printf("INFO: import complete (%d files restored), restarting", len(entries))
		os.Exit(0)
	}()
}
