# bash completion for copy command
# SPDX-License-Identifier: ISC

_copy_completion()
{
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    # Options for the copy command
    opts="-h -n -q -s -t -u -v"

    local datastores_dst="running-config startup-config"
    local datastores_src="factory-config operational-state running-config"

    case "${prev}" in
        -t)
            # Timeout expects a number, no completion
            return 0
            ;;
        -u)
            # Username, could complete with getent passwd but keep it simple
            return 0
            ;;
    esac

    # If current word starts with -, complete options
    if [[ ${cur} == -* ]]; then
        COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
        return 0
    fi

    # Determine position (source or destination)
    # Count non-option arguments before current position
    local arg_count=0
    local i
    for ((i=1; i < COMP_CWORD; i++)); do
        case "${COMP_WORDS[i]}" in
            -h|-n|-q|-s|-v)
                # Flag without argument
                ;;
            -t|-u)
                # Flag with argument, skip next word
                ((i++))
                ;;
            -*)
                # Unknown flag, might have argument
                ;;
            *)
                # Non-option argument
                ((arg_count++))
                ;;
        esac
    done

    # Helper function to add non-hidden files/dirs, filtering out dotfiles unless explicitly requested
    _add_files() {
        local IFS=$'\n'
        local files

        # If user is explicitly typing a dotfile path, include dotfiles
        if [[ ${cur} == .* || ${cur} == */.* ]]; then
            files=( $(compgen -f -- "${cur}") )
        else
            # Only show non-hidden files and directories
            files=( $(compgen -f -- "${cur}" | grep -v '/\.[^/]*$' | grep -v '^\.[^/]') )
        fi

        COMPREPLY+=( "${files[@]}" )
    }

    case ${arg_count} in
        0)
            # First argument (source): complete with files and all datastores including factory-config
            COMPREPLY=( $(compgen -W "${datastores_src}" -- ${cur}) )
            _add_files
            ;;
        1)
            # Second argument (destination): complete with files and limited datastores (no factory-config)
            COMPREPLY=( $(compgen -W "${datastores_dst}" -- ${cur}) )
            _add_files
            ;;
        *)
            # No more arguments expected
            return 0
            ;;
    esac

    return 0
}

complete -F _copy_completion copy
