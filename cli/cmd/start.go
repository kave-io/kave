package cmd

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var isDaemon bool

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Kave daemon",
	Run: func(cmd *cobra.Command, args []string) {
		// Step 1: Check if we are already the background child
		isChild, _ := cmd.Flags().GetBool("daemon-internal")

		if !isChild {
			// PARENT PROCESS: Spawn the child and exit
			runDaemon()
			return
		}

		// CHILD PROCESS (The actual Daemon logic starts here)
		serve()
	},
}

func init() {
	// Public flag for the user
	startCmd.Flags().BoolVarP(&isDaemon, "daemon", "d", false, "Run Kave in the background")

	// Hidden flag used internally for the re-exec logic
	startCmd.Flags().Bool("daemon-internal", false, "")
	startCmd.Flags().MarkHidden("daemon-internal")

	rootCmd.AddCommand(startCmd)
}

func runDaemon() {
	if daemonRunning() {
		fmt.Println("Kave daemon is already running.")
		return
	}
	// Prepare the command to run itself again with the hidden flag
	args := append(os.Args[1:], "--daemon-internal")
	child := exec.Command(os.Args[0], args...)

	// Create log file in ~/.kave/kave.log
	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, ".kave", "kave.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal(err)
	}

	// Redirect stdout and stderr to the log file
	child.Stdout = logFile
	child.Stderr = logFile

	// Start the child process
	if err := child.Start(); err != nil {
		fmt.Printf("Error starting daemon: %v\n", err)
		return
	}

	fmt.Printf("Kave daemon started (PID: %d)\n", child.Process.Pid)
	fmt.Printf("Logs: %s\n", logPath)

	// Parent exits here, leaving the child running in the background
	os.Exit(0)
}

func serve() {
	socketPath := apiSocketPath()
	if err := os.MkdirAll(filepath.Dir(socketPath), os.ModePerm); err != nil {
		log.Fatalf("unable to create socket directory: %v", err)
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		log.Fatalf("unable to clean socket: %v", err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("unable to listen on socket: %v", err)
	}
	defer os.Remove(socketPath)
	fmt.Printf("Daemon process initialized and listening on %s\n", socketPath)

	mux := http.NewServeMux()
	server := &http.Server{Handler: mux}
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("running"))
	})

	stopCh := make(chan struct{}, 1)
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("shutting down"))
		select {
		case stopCh <- struct{}{}:
		default:
		}
		go func() {
			if err := server.Shutdown(context.Background()); err != nil && err != http.ErrServerClosed {
				log.Printf("daemon shutdown error: %v", err)
			}
		}()
	})

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Serve(listener)
	}()

	<-stopCh
	if err := <-serverDone; err != nil && err != http.ErrServerClosed {
		log.Fatalf("daemon server failed: %v", err)
	}
	fmt.Println("Daemon shutdown complete.")
}
