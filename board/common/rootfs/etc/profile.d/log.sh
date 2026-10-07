# Page or follow a log file, default syslog.  Plain sh, so BusyBox ash
# and bash both get it; the bash completions live in /etc/bash.bashrc.
log()
{
	local fn="/var/log/syslog"
	[ -n "$1" ] && fn="/var/log/$1"
	less +G -r "$fn"
}

follow()
{
	local fn="/var/log/syslog"
	[ -n "$1" ] && fn="/var/log/$1"
	tail -F -n +1 "$fn"
}
