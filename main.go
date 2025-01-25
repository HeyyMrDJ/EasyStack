package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/digitalocean/go-libvirt"
	"github.com/go-chi/chi"
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

type IPAddress struct {
	IPAddressType string `json:"ip-address-type"`
	IPAddress     string `json:"ip-address"`
	Prefix        int    `json:"prefix"`
}

type NetworkInterface struct {
	Name        string      `json:"name"`
	IPAddresses []IPAddress `json:"ip-addresses"`
}

type GuestNetworkResponse struct {
	Return []NetworkInterface `json:"return"`
}

type VM struct {
	Name    string
	CPU     uint16
	RAM     uint64
	Status  string
	Console string
	IP      string
}
type status int

const (
	STOPPED status = iota
	RUNNING        = iota
)

func main() {
	defer client.Disconnect()

	if len(os.Args) < 2 {
		println("Must enter 'create', 'list', 'delete', or 'web'")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "create":
		createVM(client, os.Args[2])
	case "list":
		getVMs(client)
	case "delete":
		deleteVM(os.Args[2])
	case "web":
		start_web()
	default:
		println("Must enter 'create', 'list', 'delete', or 'web'")
		os.Exit(1)
	}
}

func start_web() {
	router := chi.NewRouter()

	fs := http.FileServer(http.Dir("static"))
	router.Handle("/static/*", http.StripPrefix("/static/", fs))

	// Route for /vm (handles GET and POST)
	router.Get("/", serveDashboard)
	router.Get("/api/vm", getVMHandler)
	router.Post("/api/vm", postVMHandler)
	router.Post("/api/vm/{name}", deleteVMHandler)
	router.Get("/vm", serveVM)
	router.Get("/storage", serveStorage)
	router.Get("/networks", serveNetworks)
	router.Get("/containers", serveContainers)

	fmt.Println("Listening on:", HTTP_PORT)
	http.ListenAndServe(HTTP_PORT, router)
}

func getVMHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("USING: ", r.Method)
	switch r.Method {
	case http.MethodGet:
		fmt.Println("GET")
		vms := getVMs(client)
		vmsJson, err := json.Marshal(vms)
		if err != nil {
			log.Fatalf("Error converting to JSON: %v", err)
		}
		fmt.Fprintln(w, string(vmsJson))
	case http.MethodPost:
		fmt.Println("POST")
		// Parse form data
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Error parsing form", http.StatusBadRequest)
			return
		}

		// Get the 'name' from the form
		name := r.FormValue("name")
		createVM(client, name)
		http.Redirect(w, r, "/", http.StatusPermanentRedirect)
	case http.MethodDelete:
		fmt.Println("DELETE")
		path := r.PathValue("name")
		deleteVM(path)
	}
}

func postVMHandler(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	// Get the 'name' from the form
	name := r.FormValue("name")
	createVM(client, name)

	// Redirect after successful VM creation
	http.Redirect(w, r, "/", http.StatusSeeOther) // Use 303 for POST redirects
}

func deleteVMHandler(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	fmt.Println("DELETE HIT", name)
	deleteVM(name)
	http.Redirect(w, r, "/", http.StatusSeeOther) // Use 303 for POST redirects
}

func serveVM(w http.ResponseWriter, r *http.Request) {
	// Create a template
	tmpl, err := template.ParseFiles("templates/index.html", "templates/vms.html")
	if err != nil {
		panic(err)
	}
	vms := getVMs(client)
	tmpl.Execute(w, vms)
}

func getVMs(client *libvirt.Libvirt) []VM {
	domains, err := client.Domains()
	if err != nil {
		fmt.Printf("Failed to get domains:  %s", err.Error())
	}

	VMs := []VM{}
	println("Getting list of VMs")
	for _, domain := range domains {
		one, _, three, four, _, _ := client.DomainGetInfo(domain)
		fmt.Printf("VM: %s\n", domain.Name)
		var mystatus string
		if one == 1 {
			mystatus = "RUNNING"
		} else {
			mystatus = "STOPPED"
		}
		command := fmt.Sprintf("virsh --connect qemu:///system qemu-agent-command %s '{\"execute\":\"guest-network-get-interfaces\"}'", domain.Name)

		output, err := sshCommand(SSH_USER, SSH_HOST, KEYPATH, command)
		if err != nil {
			log.Printf("Error running remote command: %v\nOutput: %s", err, output)
		}
		var response GuestNetworkResponse
		err = json.Unmarshal([]byte(output), &response)
		if err != nil {
			log.Printf("Error unmarshaling JSON: %v", err)
		}

		// Iterate over the interfaces and find the second IP address of enp1s0
		var ip string
		for _, iface := range response.Return {
			if iface.Name == "enp1s0" && len(iface.IPAddresses) > 1 {
				// Get the second IP address (index 1)
				ip = iface.IPAddresses[0].IPAddress
				fmt.Println("Second IP address of enp1s0:", ip)
				continue
			}
		}
		command = fmt.Sprintf("virsh --connect qemu:///system domdisplay %s", domain.Name)

		output, err = sshCommand(SSH_USER, SSH_HOST, KEYPATH, command)
		if err != nil {
			log.Printf("Error running remote command: %v\nOutput: %s", err, output)
		}
		// Split the string by "localhost:"
		output2 := strings.Split(output, "localhost:")
		port := strings.TrimSpace(output2[1])
		fmt.Println(port)
		vm := VM{domain.Name, four, three / 1048576, mystatus, port, ip}
		VMs = append(VMs, vm)
	}
	return VMs
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
    <video>
        <model type='vga'/>
    </video>
     <graphics type='spice' port='-1' autoport='yes' listen='0.0.0.0' keymap='de' defaultMode='insecure'>
      <listen type='address' address='0.0.0.0'/>
      <image compression='off'/>
      <gl enable='no'/>
    </graphics>
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

func serveContainers(w http.ResponseWriter, r *http.Request) {
	// Create a template
	tmpl, err := template.ParseFiles("templates/index.html", "templates/containers.html")
	if err != nil {
		panic(err)
	}
	vms := getVMs(client)
	tmpl.Execute(w, vms)
}

func serveStorage(w http.ResponseWriter, r *http.Request) {
	// Create a template
	tmpl, err := template.ParseFiles("templates/index.html", "templates/storage.html")
	if err != nil {
		panic(err)
	}
	vms := getVMs(client)
	tmpl.Execute(w, vms)
}

func serveNetworks(w http.ResponseWriter, r *http.Request) {
	// Create a template
	tmpl, err := template.ParseFiles("templates/index.html", "templates/network.html")
	if err != nil {
		panic(err)
	}
	vms := getVMs(client)
	tmpl.Execute(w, vms)
}

func serveDashboard(w http.ResponseWriter, r *http.Request) {
	// Create a template
	tmpl, err := template.ParseFiles("templates/index.html", "templates/dashboard.html")
	if err != nil {
		panic(err)
	}
	vms := getVMs(client)
	tmpl.Execute(w, vms)
}
