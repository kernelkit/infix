if button factory-reset; then
    echo "Keep button pressed for 10 seconds to engage reset"
    if sleep 10 && button factory-reset; then
	setenv factory_reset yes
	echo "FACTORY RESET ENGAGED"
    fi
fi
