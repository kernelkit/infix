# Text Editor

The CLI built-in `text-editor` command can be used to edit type `binary`
settings in configure context.

The default editor is a Micro Emacs clone.  Users not familiar with
terminal based editors may benefit from this introduction.


## Escape Meta/Alt Control Shift

When starting up, the editor status field at the bottom shows the
following shorthand:

```
C-h q  quick help | C-h t  tutorial | C-h b  key bindings | C = Ctrl | M = Alt
```

Key combinations with a `-` (dash) mean holding down the modifier key.
Combinations without a `-` (dash) mean without any modifier key.

### Quick help `C-h q`

  - hold down the `Ctrl` key on
  - tap the `h` key
  - release `Ctrl`
  - tap the `q` key

The bottom part of the terminal now shows a "buffer" called `*quick*`:

```
FILE              BUFFER           WINDOW            MARK/KILL        MISC
C-x C-c exit      C-x b   switch   C-x 0 only other  C-space mark     C-_ undo
C-x C-f find      C-x k   close    C-x 1 only this   C-w     kill-rg  C-s search
C-x C-s save      C-x C-b list     C-x 2 split two   C-k     kill-ln  C-r r-search
C-x s   save-all  C-x h   mark     C-x ^ enlarge     C-y     yank     M-% replace
C-x i   insert    C-x g   goto-ln  C-x o other win   C-x C-x swap     M-q reformat
```

### Save & Exit `C-x C-c`

  - Hold down the Ctrl key
  - tap `X`
  - tap `c`
  - release `Ctrl`

> The status field at the bottom asks if you are really sure, and/or if
> you want to add a final Enter/newline to the file.  For binary content
> that final newline may be important.


## Changing the Editor

The system has three different built-in editors: 

 - `emacs` (Micro Emacs)
 - `nano` (GNU Nano)
 - `vi` (Visual Editor)

Changing editor is done in configure context, in the system container:

```
admin@host:/> configure
admin@host:/config/> edit system
admin@host:/config/system/> set text-editor <TAB>
emacs	nano	vi
admin@host:/config/system/> set text-editor nano
admin@host:/config/system/> leave
admin@example:/> 
```
