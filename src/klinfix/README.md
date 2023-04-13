Infix Extensions to klish-plugin-sysrepo
----------------------------------------

Tailor `klish` and `klish-plugin-sysrepo` to suit Infix. The resulting
CLI is, in many respects, still very similar to Juniper's JunOS, since
that is also the primary inspiration for klish, with some additions
and modifications.

Broadly speaking, the CLI is split up in two major modes:

- Admin/Exec: This is the default mode, where operational commands can
  be issued.
- Configure: Where a new candidate configuration is prepared.


### Admin/Exec Mode

The following table lists the available commands:

| Command            | Description                           | Hotkey |
|--------------------|---------------------------------------|--------|
| `configure`        | Enter configuration mode              |        |
| `copy <src> <dst>` | Copy `<src>` to `<dst>`               |        |
| `exit`             | Exit                                  | `C-d`  |
| `logout`           | Alias for `exit`                      |        |
| `shell`            | Start system shell                    |        |
| `show <item>`      | Show various configuration and status |        |

#### Copy Command

`copy <src> <dst>`

Where `<src>` is one of:
- `factory-config`
- `startup-config`
- `running-config`

And `<dst>` is one of:
- `startup-config`
- `running-config`

#### Show Command

`show <item>`

The following table lists the available items:

| Item             | Description                          |
|------------------|--------------------------------------|
| `running-config` | Show the active system configuration |
|                  |                                      |


### Configure

The following table lists the available commands:

| Command    | Description                                                  | Hotkey |
|------------|--------------------------------------------------------------|--------|
| `abort`    | Abandon candidate                                            | `C-d`  |
| `check`    | Validate candidate                                           |        |
| `commit`   | Commit current candidate to running-config                   |        |
| `delete`   | Delete configuration setting(s)                              |        |
| `diff`     | Summarize uncommitted changes                                |        |
| `do`       | Execute operational mode command                             |        |
| `edit`     | Descend to the specified configuration node                  |        |
| `exit`     | Ascend to the parent configuration node, or abort (from top) |        |
| `leave`    | Finalize candidate and apply to running-config               | `C-z`  |
| `no`       | Alias for delete                                             |        |
| `rollback` | Restore candidate to running-config                          |        |
| `set`      | Set configuration setting                                    |        |
| `show`     | Show configuration                                           |        |
| `top`      | Ascend to the configuration root                             |        |
| `up`       | Ascend to the parent configuration node                      | `C-c`  |
