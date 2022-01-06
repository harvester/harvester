#!/bin/bash
set -e

FAIL(){
    echo -e "\033[31;1m"FAIL"\033[0m"
}
PASS(){
    echo -e "\033[32;1m"PASS"\033[0m"
}
WARN(){
    echo -e "\033[33;1m"WARN"\033[0m"
}

for i in iscsiadm mount.nfs4 bash curl findmnt grep awk blkid lsblk; do
    printf "    LONGHORN: Checking for command '$i' exists: "
    if which "$i" >/dev/null; then
	    PASS
    else
	    FAIL
    fi
done

printf "    QEMU: Checking for hardware virtualization: "
if egrep '(vmx|svm)' /proc/cpuinfo >/dev/null ; then
    PASS
else
    FAIL
fi

for i in /dev/kvm /dev/vhost-net /dev/net/tun; do
    printf "    QEMU: Checking for device '$i' exists: "
    if file "$i" >/dev/null; then
	    PASS
    else
	    FAIL
    fi
done


kernel_main_version=`uname -r | awk -F '.' '{print $1}'`
printf "    KERNEL: Checking for kernel version > 3.x: "
if [[ ${kernel_main_version} -gt 3 ]];then
	PASS
else
	FAIL
fi
