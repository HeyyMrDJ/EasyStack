package main

import (
	"fmt"
	"os/exec"
	"log"
)

func main() {
	cmd := exec.Command("sh","-c", "sudo virt-install -n ubuntu-vm3 --connect qemu:///system --description Ubuntu --os-type=Linux --ram=1024 --vcpus=2 --disk path=/var/lib/libvirt/images/ubuntu-vm3.img,bus=virtio,size=4 --graphics spice --noautoconsole --location /home/jadmin/ubuntu14.iso --extra-args='console=tty0 console=ttyS0,115200n8 serial' --network bridge:br0")
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", stdoutStderr)
}
