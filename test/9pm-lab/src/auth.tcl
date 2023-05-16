#!/usr/bin/env tclsh

package require 9pm
source "[9pm::misc::get::script_path]/../lib/infix.tcl"

9pm::output::plan 8

set CNT 20
set infix [dict get $9pm::conf::data "infix"]

9pm::shell::open "target"
9pm::ssh::connect $infix
9pm::output::info "Connected to Infix"

# TODO: Remove once admin is admin.
send "su -l root\n"
expect {
    "root@[dict get $infix "prompt"]" {}
    default { 9pm::fatal ::9pm::output::fail "Unable to su root" }
}

9pm::output::info "Creating $CNT users without password"
infix::cli::run_code $infix {
    for {set i 0} {$i < $CNT} {incr i} {
        infix::cli::run $infix "set system authentication user user$i"
    }
}
9pm::output::ok "Created $CNT new users"

9pm::output::info "Checking that all $CNT users are in shadow"
for {set i 0} {$i < $CNT} {incr i} {
    9pm::cmd::execute "grep 'user$i:!:' /etc/shadow" 0
}
9pm::output::ok "All $CNT users are passwordless in shadow"

9pm::output::info "Setting password for all $CNT users"
infix::cli::run_code $infix {
    for {set i 0} {$i < $CNT} {incr i} {
        infix::cli::run $infix \
            "set system authentication user user$i password \$1\$AxkhipBg\$fxSMhuY.3EQyrfJlKmEDN1"
    }
}
9pm::output::ok "All $CNT users now have password"

9pm::output::info "Checking that all $CNT users has password in shadow"
for {set i 0} {$i < $CNT} {incr i} {
    9pm::cmd::execute "grep 'user$i:\$1\$AxkhipBg\$fxSMhuY.3EQyrfJlKmEDN1:' /etc/shadow" 0
}
9pm::output::ok "All $CNT users has a password in shadow"

9pm::output::info "Removing password for $CNT users"
infix::cli::run_code $infix {
    for {set i 0} {$i < $CNT} {incr i} {
        infix::cli::run $infix \
            "delete system authentication user user$i password"
    }
}
9pm::output::ok "All $CNT users now lack password"

9pm::output::info "Checking that all $CNT users passwords are gone in shadow"
for {set i 0} {$i < $CNT} {incr i} {
    9pm::cmd::execute "grep 'user$i:!:' /etc/shadow" 0
}
9pm::output::ok "All $CNT users are passwordless in shadow"

9pm::output::info "Removing $CNT users"
infix::cli::run_code $infix {
    for {set i 0} {$i < $CNT} {incr i} {
        infix::cli::run $infix \
            "delete system authentication user user$i"
    }
}
9pm::output::ok "All $CNT users now lack password"

9pm::output::info "Checking that all users are removed from shadow"
for {set i 0} {$i < $CNT} {incr i} {
    9pm::cmd::execute "grep 'user$i:' /etc/shadow" 1
}
9pm::output::ok "All $CNT users are removed from shadow"
