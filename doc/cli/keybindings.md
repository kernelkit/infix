# Keybindings

Writing CLI commands by hand is very tedious.  To make things easier the
CLI has several keybindings, most significant first:

| **Key** | **Cmd/Key**    | **Description**                                  |
|---------|----------------|--------------------------------------------------|
| TAB     |                | Complete current command, see below for example  |
| ?       |                | Show available commands, or arguments, with help |
| Ctrl-c  |                | Cancel everything on current line                |
| Ctrl-d  | `abort`/`exit` | Delete character, or abort/exit on empty line    |
| Ctrl-z  | `leave`        | Leave and activate changes in configure context  |
| Ctrl-f  | Right arrow    | Move cursor forward one character                |
| Ctrl-b  | Left arrow     | Move cursor back one character                   |
| Meta-f  | Ctrl-Right     | Move cursor forward one word                     |
| Meta-b  | Ctrl-Left      | Move cursor back one word                        |
| Ctrl-e  | End            | Move cursor to end of line                       |
| Ctrl-a  | Home           | Move cursor to beginning of line                 |
| Ctrl-k  |                | Kill (cut) text from cursor to end of line       |
| Ctrl-u  |                | Delete (cut) entire line                         |
| Ctrl-y  |                | Yank (paste) from kill buffer to cursor          |
| Meta-.  |                | Yank (paste) last argument from previous line    |
| Ctrl-w  | Meta-Backspace | Delete (cut) word to the left                    |
|         | Meta-Delete    | Delete (cut) word to the right                   |
| Ctrl-l  |                | Clear screen and refresh current line            |
| Ctrl-p  | Up arrow       | History, previous command                        |
| Ctrl-n  | Down arrow     | History, next command                            |
| Ctrl-r  |                | History, reversed interactive search (i-search)  |

> **Note:** the Meta key is called Alt on most modern keyboards.  If you
> have neither, first tap the Esc key instead of holding down Alt/Meta.

## Examples

Complete a word.  Start by typing a few characters, then tap the TAB key
on your keyboard:

    conf<TAB> --> configure

See possible arguments, with brief help text, to a command:

    show ?
    bridge          Show bridge (ports/fdb/mdb/vlans)
    datetime        Show current date and time, default RFC2822 format
    ...

Type the command, then tap the `?` key.

