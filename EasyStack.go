package main

import (
	"fmt"
	"os"
libvirt "libvirt.org/libvirt-go"
)

var DB *libvirt.Connect

func main() {


	input := GetInput()
	GetRunningVMs(input)
}

func OpenRemoteConnection(i string) *libvirt.Connect {
	connection := "qemu+ssh://" + i + "/system?socket=/var/run/libvirt/libvirt-sock"
	conn, err := libvirt.NewConnect(connection)
	if err != nil {
		fmt.Println(err)
		os.Exit(0)
	}
	
	return conn
}

func GetRunningVMs(input string){
	conn := OpenRemoteConnection(input)
	doms, err := conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		fmt.Println(err,doms)
	}
	
	fmt.Printf("Running Domains: %d\n", len(doms))
	for _, dom := range doms {
		name, err := dom.GetName()
		if err == nil {
			fmt.Printf("  Name: %s\n", name)
		}
		dom.Free()
	}
}

func GetInput() string{
	var input string
	fmt.Println("Enter the username and Hostname/IP address of the remote system. (eg. admin@host1)")
	fmt.Scanln(&input)
	return input
}