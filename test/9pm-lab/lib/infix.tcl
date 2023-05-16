namespace eval ::infix::cli {
    proc exp_prompt { prompt } {
        expect {
            -re {Error: [^\r\n]+} {
                9pm::output::fail "$expect_out(0,string)"
                exp_continue -continue_timer
            }
            $prompt { }
            default { 9pm::fatal 9pm::output::fail "Error, expecting prompt ($prompt)" }
        }
    }
    proc start { node } {
        9pm::cmd::start "cli"
        expect {
            "[dict get $node "prompt"]:exec>" { 9pm::output::debug "Infix CLI started" }
            default { 9pm::fatal 9pm::output::fail "Error, starting Infix CLI" }
        }

    }
    proc config { node } {
        send "configure\r"
        exp_prompt "[dict get $node "prompt"]:configure>"
        9pm::output::debug "CLI entered configure mode"
    }
    proc leave { node } {
        send "leave\r"
        exp_prompt "[dict get $node "prompt"]:exec>"
        9pm::output::debug "CLI left configure mode"
    }
    proc exit { node } {
        send "exit\r"
        9pm::cmd::finish
        9pm::output::debug "CLI closed"
    }
    proc run { node cmd } {
        expect *
        send "$cmd\r"
        expect {
            -re {Error: [^\r\n]+} {
                9pm::output::fail "$expect_out(0,string)"
                exp_continue -continue_timer
            }
            {[Edit]} { }
            default { 9pm::fatal 9pm::output::fail "Error, running $cmd" }
        }
        exp_prompt "[dict get $node "prompt"]:configure>"
    }
    proc run_code { node code} {
        infix::cli::start $node
        infix::cli::config $node
        uplevel 1 $code
        infix::cli::leave $node
        9pm::output::debug "Code successfully executed in CLI"
        infix::cli::exit $node
    }
}
