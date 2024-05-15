---
name: "\U0001F41E Bug report"
about: Create a bug report
title: "[BUG]"
labels: bug, triage
assignees: ''

---

For Q&A and support, please use the [Discussion forum][1].

# Description

## Steps to reproduce the problem

 1. Built from source

 2. Upgraded to latest release, vYY.MM.P

 3. Factory reset

 4. Enable feature
 
 5. Check the logs/show command/operational status

## Current outcome

For code snippets, logs, commands, etc., please use triple backticks:

```
admin@infix-c0-ff-ee:/> show log
...
May 15 07:21:02 infix-00-00-00 container[3192]: Failed creating container test from curios-httpd-v24.03.0
...
- (press h for help or q to quit)
```


## Expected outcome

Describe or show

```
admin@infix-c0-ff-ee:/> show log
...
May 15 07:21:02 infix-c0-ff-ee container[3192]: Successfully created container test from curios-httpd-v24.03.0
...
- (press h for help or q to quit)
```


# Additional information

 - Relevant parts of `startup-config`
 - Output from `show interfaces`, if applicable
 - Other observations
 - Screenshots

[1]: https://github.com/kernelkit/infix/discussions
