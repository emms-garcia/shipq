//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/emms-garcia/shipq/internal/domain/identity"
	"github.com/emms-garcia/shipq/internal/host"
	"github.com/emms-garcia/shipq/tests/support/scenario"
)

type handoff struct {
	SocketPath        string   `json:"socket_path"`
	ControlSocketPath string   `json:"control_socket_path"`
	ProfileID         string   `json:"profile_id"`
	ScenarioIDs       []string `json:"scenario_ids"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fixture host unavailable")
		os.Exit(1)
	}
}

func run() error {
	root, err := os.MkdirTemp("/tmp", "shipq-fixture-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	driver, err := scenario.Load(filepath.Join("tests", "fixtures", "scenarios", "pr_management.json"))
	if err != nil {
		return err
	}
	profileID, err := identity.ParseID("44444444-4444-4444-8444-444444444444")
	if err != nil {
		return err
	}
	runtime, err := host.Start(context.Background(), host.Config{
		DataRoot: filepath.Join(root, "profiles"), RuntimeDirectory: filepath.Join(root, "api"),
		ProfileID: profileID, Status: host.StaticStatus(profileID),
	})
	if err != nil {
		return err
	}
	control, controlPath, err := startControl(filepath.Join(root, "control"), driver)
	if err != nil {
		_ = runtime.Shutdown(context.Background())
		return err
	}
	defer control.Close()

	value := handoff{SocketPath: runtime.SocketPath(), ControlSocketPath: controlPath, ProfileID: profileID.String(), ScenarioIDs: driver.ScenarioIDs()}
	handoffPath := os.Getenv("SHIPQ_FIXTURE_HANDOFF")
	if handoffPath == "" {
		handoffPath = filepath.Join("build", "fixture", "connection.json")
	}
	if !filepath.IsAbs(handoffPath) {
		absolute, err := filepath.Abs(handoffPath)
		if err != nil {
			return err
		}
		handoffPath = absolute
	}
	if err := writeHandoff(handoffPath, value); err != nil {
		return err
	}
	defer os.Remove(handoffPath)
	encoded, _ := json.Marshal(value)
	fmt.Println(string(encoded))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return errors.Join(control.Shutdown(shutdown), runtime.Shutdown(shutdown))
}

func writeHandoff(path string, value handoff) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func startControl(directory string, driver *scenario.Driver) (*http.Server, string, error) {
	if err := os.Mkdir(directory, 0o700); err != nil {
		return nil, "", err
	}
	path := filepath.Join(directory, "control.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		return nil, "", err
	}
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/state":
			_ = json.NewEncoder(response).Encode(map[string]any{"now": driver.Now(), "scenario_ids": driver.ScenarioIDs()})
		case request.Method == http.MethodPost && request.URL.Path == "/advance":
			var input struct {
				ScenarioID string `json:"scenario_id"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 4096))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&input) != nil || input.ScenarioID == "" {
				http.Error(response, `{"error":"invalid_request"}`, http.StatusBadRequest)
				return
			}
			step, err := driver.Advance(input.ScenarioID)
			if err != nil {
				http.Error(response, `{"error":"invalid_step"}`, http.StatusConflict)
				return
			}
			_ = json.NewEncoder(response).Encode(step)
		case request.Method == http.MethodPost && request.URL.Path == "/reset":
			driver.Reset()
			_ = json.NewEncoder(response).Encode(map[string]bool{"reset": true})
		default:
			http.Error(response, `{"error":"not_found"}`, http.StatusNotFound)
		}
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	go server.Serve(listener)
	return server, path, nil
}
