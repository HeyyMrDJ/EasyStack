package main

import (
	"fmt"
	"log"
	"net"

	"github.com/digitalocean/go-libvirt"
)

func main() {
	// Replace with the IP address or hostname of your remote libvirt server
	const remoteAddr = "192.168.201.213:16509"

	conn, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		log.Fatalf("Failed to connect to libvirt: %v", err)
	}
	defer conn.Close()

	l := libvirt.New(conn)
	if err := l.Connect(); err != nil {
		log.Fatalf("Failed to authenticate: %v", err)
	}

	defer l.Disconnect()

	// Example: List all domains
	domains, err := l.Domains()
	if err != nil {
		log.Fatalf("Failed to list domains: %v", err)
	}

	println("Getting list of VMs")
	for _, domain := range domains {
		fmt.Printf("VM: %s\n", domain.Name)
	}
}
