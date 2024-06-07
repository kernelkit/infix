if button factory-reset; then
    echo "Keep button pressed for 10 seconds to engage factory reset"
    if sleep 10 && button factory-reset; then
	run ixfactory
    fi
fi
