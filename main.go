package main

import (
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/digitalocean/go-libvirt"
)

var client = createConnection()

const (
	LIBVIRT_PORT = "16509" // Default libvirtd port
	HTTP_PORT    = ":8089"
	SSH_PORT     = "22"
	BASEIMAGE    = "/var/lib/libvirt/images/Fedora.tmpl"
	TIMEOUT      = 10 * time.Second // Connection timeout
)

var HOST = os.Getenv("HOST") // Remote server address
var SSH_USER = os.Getenv("SSH_USER")
var KEYPATH = os.Getenv("KEYPATH")
var SSH_HOST = HOST + ":" + SSH_PORT

func main() {
	defer client.Disconnect()

	switch os.Args[1] {
	case "create":
		createVM(client, os.Args[2])
	case "list":
		getVMs(client)
	case "delete":
		deleteVM(os.Args[2])
	}

	// Serve static files (CSS, JS, Images, etc.)
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Handler function for the root path ("/")
	http.HandleFunc("/", serve_home)

	// Start the server listening on port 8080
	fmt.Printf("Server listening on http://localhost%s/", HTTP_PORT)
	http.ListenAndServe(HTTP_PORT, nil)
}

func serve_home(w http.ResponseWriter, r *http.Request) {
	// Create a template
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		panic(err)
	}
	vms := getVMs(client)
	tmpl.Execute(w, vms)
}

func getVMs(client *libvirt.Libvirt) []string {
	domains, err := client.Domains()
	if err != nil {
		fmt.Printf("Failed to get domains:  %s", err.Error())
	}

	vms := []string{}
	println("Getting list of VMs")
	for _, domain := range domains {
		fmt.Printf("VM: %s\n", domain.Name)
		vms = append(vms, domain.Name)
	}
	return vms
}

func createVM(client *libvirt.Libvirt, name string) {
	newImage := fmt.Sprintf("/var/lib/libvirt/images/%s.qcow2", name)

	// Command to clone the image
	command := fmt.Sprintf("qemu-img create -f qcow2 -F qcow2 -b %s %s", BASEIMAGE, newImage)

	output, err := sshCommand(SSH_USER, SSH_HOST, KEYPATH, command)
	if err != nil {
		log.Fatalf("Error running remote command: %v\nOutput: %s", err, output)
	}
	// Command to clone the image
	command = fmt.Sprintf("echo local-hostname: %s > /var/lib/libvirt/images/cloud-init-iso/cloud-init/meta-data", name)

	output, err = sshCommand(SSH_USER, SSH_HOST, KEYPATH, command)
	if err != nil {
		log.Fatalf("Error running remote command: %v\nOutput: %s", err, output)
	}

	command = fmt.Sprintf("cd /var/lib/libvirt/images/cloud-init-iso && xorriso -as mkisofs -o cloud-init-%s.iso -J -R -V \"cidata\" cloud-init", name)

	output, err = sshCommand(SSH_USER, SSH_HOST, KEYPATH, command)
	if err != nil {
		log.Fatalf("Error running remote command: %v\nOutput: %s", err, output)
	}

	// XML based on your working configuration
	domainXML := fmt.Sprintf(`
<domain type='kvm'>
  <name>%s</name>
  <memory unit='KiB'>2097152</memory>
  <currentMemory unit='KiB'>2097152</currentMemory>
  <vcpu placement='static'>2</vcpu>
  <os>
    <type arch='x86_64' machine='pc-q35-7.2'>hvm</type>
    <boot dev='hd'/>
  </os>
  <features>
    <acpi/>
    <apic/>
    <vmport state='off'/>
  </features>
  <devices>
    <!-- Cloud-Init ISO attachment -->
    <disk type='file' device='disk'>
      <source file='/var/lib/libvirt/images/cloud-init-iso/cloud-init-%s.iso'/>
      <target dev='vdb' bus='virtio'/>
    </disk>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2'/>
      <source file='/var/lib/libvirt/images/%s.qcow2'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <interface type='bridge'>
      <source bridge='br0'/>
      <model type='virtio'/>
    </interface>
    <console type='pty'>
      <target type='serial' port='0'/>
    </console>
      <channel type='unix'>
        <target type='virtio' name='org.qemu.guest_agent.0'/>
      </channel>
  </devices>
</domain>`, name, name, name)

	dom, err := client.DomainDefineXML(domainXML)
	if err != nil {
		println("Error:", err.Error())
	} else {
		fmt.Println("VM Created:", name)
	}
	err = client.DomainCreate(dom)
	if err != nil {
		fmt.Println("FAIL STARTING: ", err.Error())
	} else {
		fmt.Println("Started VM:", name)
	}
}

func deleteVM(name string) {
	command := fmt.Sprintf("virsh --connect qemu:///system destroy %s", name)

	output, err := sshCommand(SSH_USER, SSH_HOST, KEYPATH, command)
	if err != nil {
		log.Printf("Error running remote command: %v\nOutput: %s", err, output)
	}

	command = fmt.Sprintf("virsh --connect qemu:///system undefine %s", name)

	output, err = sshCommand(SSH_USER, SSH_HOST, KEYPATH, command)
	if err != nil {
		log.Fatalf("Error running remote command: %v\nOutput: %s", err, output)
	}

	command = fmt.Sprintf("rm /var/lib/libvirt/images/%s.qcow2", name)

	output, err = sshCommand(SSH_USER, SSH_HOST, KEYPATH, command)
	if err != nil {
		log.Fatalf("Error running remote command: %v\nOutput: %s", err, output)
	}
	command = fmt.Sprintf("rm /var/lib/libvirt/images/cloud-init-iso/cloud-init-%s.iso", name)

	output, err = sshCommand(SSH_USER, SSH_HOST, KEYPATH, command)
	if err != nil {
		log.Fatalf("Error running remote command: %v\nOutput: %s", err, output)
	}

}

func createConnection() *libvirt.Libvirt {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(HOST, LIBVIRT_PORT), TIMEOUT)
	if err != nil {
		println(err.Error())
	}

	// Initialize the libvirt client
	client := libvirt.New(conn)
	if err := client.Connect(); err != nil {
		log.Fatalf("Failed to authenticate with libvirt: %v", err)
	}
	return client
}

func sshCommand(user, host, keyPath, command string) (string, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return "", fmt.Errorf("unable to read private key: %v", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("unable to parse private key: %v", err)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return "", fmt.Errorf("failed to connect to %s: %v", host, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	return string(output), err
}
