alias factory='read -N 1 -p "Factory reset requires a reboot, continue (y/N)? " yorn; echo; if [ "$yorn" = "y" ]; then touch /rw/infix/.reset; reboot; fi'
