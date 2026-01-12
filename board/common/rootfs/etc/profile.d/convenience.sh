alias l='ls -CF'
alias la='ls -A'
alias ll='ls -alF'
alias ls='ls --color=auto'

export LANG=C.UTF-8
export EDITOR=/usr/bin/editor
export VISUAL=/usr/bin/editor
export LESS="-P %f (press h for help or q to quit)"
export LESSOPEN="|/usr/bin/lesspipe.sh %s"
alias vim='vi'
alias view='vi -R'
alias emacs='mg'
alias sensible-editor=editor
alias sensible-pager=pager
alias hd="hexdump -C"

alias ip='ip --color=auto'
alias ipb='ip -br'
alias ipaddr='ip addr'
alias iplink='ip link'
alias bridge='bridge --color=auto'

alias llping='ping -L ff02::1 -I'

alias docker=podman
