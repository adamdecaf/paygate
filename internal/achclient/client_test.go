// Copyright 2020 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package achclient

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
	"github.com/moov-io/ach"
)

func TestClient__pingRoute(t *testing.T) {
	achClient, _, server := MockClientServer("pingRoute", AddPingRoute)
	defer server.Close()

	// Make our ping request
	if err := achClient.Ping(); err != nil {
		t.Fatal(err)
	}
}

func TestClient__CreateFile(t *testing.T) {
	w := httptest.NewRecorder()

	achClient, _, server := MockClientServer("fileCreate", func(r *mux.Router) {
		AddCreateRoute(w, r)
	})
	defer server.Close()

	bs, err := ioutil.ReadFile(filepath.Join("..", "..", "testdata", "ppd-valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := ach.FileFromJSON(bs)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Validate(); err != nil {
		t.Fatal(err)
	}

	id := file.ID

	fileId, err := achClient.CreateFile("unique", file)
	if err != nil {
		t.Fatal(err)
	}
	if id != fileId {
		t.Errorf("id=%s fileId=%s", id, fileId)
	}

	w.Flush()

	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}

	// Decode body we sent to ACH service
	f, err := ach.FileFromJSON(w.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	// Check body we sent
	if f.ID != "fileId" {
		t.Errorf("f.ID=%s", f.ID)
	}
	if f.Header.ID != "fileId" {
		t.Errorf("f.Header.ID=%v", f.Header.ID)
	}
	if len(f.Batches) != 1 {
		t.Errorf("got %d batches", len(f.Batches))
		for i := range f.Batches {
			t.Errorf("  batch[%d]=%#v", i, f.Batches[i])
		}
	}
	if f.Control.ID != "fileId" {
		t.Errorf("f.Control.ID=%v", f.Control.ID)
	}

	// Check the batch
	batch := f.Batches[0]
	header := batch.GetHeader()
	if header.ID != "fileId" {
		t.Errorf("Batch Header ID=%v", header.ID)
	}
	entries := batch.GetEntries()
	if len(entries) != 1 {
		t.Errorf("got %d batch EntryDetails", len(entries))
		for i := range entries {
			t.Errorf("  batch EntryDetails[%d]=%#v", i, entries[i])
		}
	}
	if batch.GetControl().ID != "" {
		t.Errorf("batch Control ID=%v", batch.GetControl().ID)
	}
}

func TestClient__DeleteFile(t *testing.T) {
	w := httptest.NewRecorder()

	achClient, _, server := MockClientServer("fileDelete", func(r *mux.Router) {
		AddCreateRoute(w, r)
		AddDeleteRoute(r)
	})
	defer server.Close()

	// Create file
	bs, err := ioutil.ReadFile(filepath.Join("..", "..", "testdata", "ppd-valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := ach.FileFromJSON(bs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := achClient.CreateFile("create", file); err != nil {
		t.Fatal(err)
	}

	// Delete File
	if err := achClient.DeleteFile("delete"); err != nil {
		t.Fatal(err)
	}
}

func TestClient__Delete404(t *testing.T) {
	achClient, _, server := MockClientServer("fileDelete404", func(r *mux.Router) {
		r.Methods("DELETE").Path("/files/{fileId}").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("{}"))
		})
	})
	defer server.Close()

	// Delete File (expect no error though)
	if err := achClient.DeleteFile("delete"); err != nil {
		t.Fatal(err)
	}
}

func TestClient__GetFile(t *testing.T) {
	achClient, _, server := MockClientServer("fileDelete", func(r *mux.Router) {
		AddGetFileRoute(r)
	})
	defer server.Close()

	file, err := achClient.GetFile("fileId")
	if err != nil || file == nil {
		t.Fatalf("file=%v err=%v", file, err)
	}

	if file.Header.ImmediateOrigin == "" {
		t.Error("empty file")
	}
}
