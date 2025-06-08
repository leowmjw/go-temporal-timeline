package main

import (
	"log"
	"os"
	"os/exec"
)

func main() {
	// Run the server from cmd/server
	cmd := exec.Command("go", "run", "./cmd/server")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
