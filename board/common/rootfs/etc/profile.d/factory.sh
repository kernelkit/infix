alias factory='read -N 1 -p "Factory reset requires a reboot, continue (y/N)? " yorn; echo; if [ "$yorn" = "y" ]; then touch /mnt/cfg/infix/.reset; reboot; fi'
