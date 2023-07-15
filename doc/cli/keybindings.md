# Keybindings

> Tap `q` or `Ctrl-c` to quit the help viewer.

Writing CLI commands by hand is very tedious.  To make things easier the
CLI has several keybindings, most significant first:

| **Key** | **Cmd/Key**   | **Description**                                  |
|---------|---------------|--------------------------------------------------|
| TAB     |               | Complete current command, see below for example  |
| ?       |               | Show available commands, or arguments, with help |
| Ctrl-c  |               | Cancel everything on current line                |
| Ctrl-d  | `abort`       | Abort context, `exit` in admin-exec              |
| Ctrl-z  | `leave`       | Leave and activate changes in configure context  |
| Ctrl-f  | Right arrow   | Move cursor forward one character                |
| Ctrl-b  | Left arrow    | Move cursor back one character                   |
| Alt-f   | Ctrl-Right    | Move cursor forward one word                     |
| Alt-b   | Ctrl-Left     | Move cursor back one word                        |
| Ctrl-e  | End           | Move cursor to end of line                       |
| Ctrl-a  | Home          | Move cursor to beginning of line                 |
| Ctrl-k  |               | Kill (cut) text from cursor to end of line       |
| Ctrl-u  |               | Delete (cut) entire line                         |
| Ctrl-y  |               | Yank (paste) from kill buffer to cursor          |
| Ctrl-w  | Alt-Backspace | Delete (cut) word to the left                    |
|         | Alt-Delete    | Delete (cut) word to the right                   |
|         |               |                                                  |

> **Note:** the Alt key on your keyboard may also be called Meta.  If you
> have neither, first tap the Esc key instead of holding down Alt/Meta.

## Examples

Complete a word.  Start by typing a few characters, then tap the TAB key
on your keyboard:

    conf<TAB> --> configure

See possible arguments, with brief help text, to a command:

    show ?

Type the command, then tap the `?` key.

