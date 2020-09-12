package main

import (
	"fmt"
	"os/exec"
	"log"
)

func main() {
	cmd := exec.Command("sh","virsh list")
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", stdoutStderr)
}
