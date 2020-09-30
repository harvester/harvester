# Access to the VM

Once the VM is up and running, it can be accessed using either VNC or serial console from the Harvester UI, optionally the user can connect it directly from the computer's SSH client.

#### Access with UI

VM can be accessed from the UI directly using either VNC or serial console, if the VGA display is not enabled of the VM(e.g, when using the Ubuntu minimal cloud image), the user can access the VM with the serial console.


#### Access using SSH

Use the address in a terminal emulation client (such as Putty) or use the following command line to access the VM directly from your computer SSH client:
```bash
 ssh -i ~/.ssh/your-ssh-key user@<ip-address-or-hostname>
```

![](./assets/access-to-vm.png)
