Package override
================

This guide demonstrates how the `local.mk` file is utilized to override 
a Linux Buildroot package. The example of `tcpdump` serves to illustrate 
this process.

In an instance such as `Infix` using tcpdump, the `local.mk` file is modified 
as shown below: 

```
TCPDUMP_OVERRIDE_SRCDIR = /path/to/tcpdump/repo
```

and stored in the `output/` folder, alongside the `.config` file:
```
user@PC:~/infix$ll output/ | grep -e 'local.mk' -e '.config'
-rw-r--r--   1 group user 119936 Nov 10 18:04 .config
-rw-r--r--   1 group user     43 Nov 10 18:25 local.mk
```

The execution of `make tcpdump-rebuild all` triggers a process where 
Buildroot synchronizes the tcpdump source code from the specified override directory 
to `output/build/tcpdump-custom`, followed by the rebuilding of the entire project. 

```
user@PC:~/infix$ make tcpdump-rebuild all
```


```
user@PC:~/infix$ ll /output/build/ | grep tcpdump
drwxr-xr-x   7 group user 20480 Nov 10 18:26 tcpdump-4.99.4/
drwxr-xr-x   7 group user 12288 Nov 10 18:28 tcpdump-custom/
```

Buildroot follows a process of downloading and processing tarballs 
(extraction, configuration, compilation, and installation). 
The source code is stored  in a temporary directory:
`output/build/<package>-<version>` (i.e. `tcpdump-4.99.4/`), 
which is removed and recreated with each `make` command. That is why 
the direct modifications in the `output/build` directory are generally 
**not recommended**. 

To manage the development changes more effectively, where the package source code 
remains untouched, Buildroot incorporates the `<pkg>_OVERRIDE_SRCDIR` feature.

For a comprehensive understanding of utilizing Buildroot during development, 
including detailed elaboration on the `<pkg>_OVERRIDE_SRCDIR` feature, 
refer to section 8.13.6 in [Using Buildroot during development](https://nightly.buildroot.org/).