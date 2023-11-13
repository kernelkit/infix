Package override
================

Buildroot typically functions by downloading a tarball, then going through the steps of extracting, configuring, compiling, and installing the software component. The source code is unpacked into a temporary directory named output/build/\<package\>-\<version\>. This directory is ephemeral; it gets completely deleted when the command 'make clean' is executed and is recreated during the next 'make' command execution. Regardless of whether the package source code comes from a Git or Subversion repository, Buildroot first converts it into a tarball and then processes it as it would with any other tarball.

This approach works well when Buildroot is primarily employed as a tool for integration. Yet, the method may not be as effective for those who use Buildroot in the development phase of specific system components. In such scenarios, developers often prefer to implement minor modifications to the source code of a single package and swiftly rebuild the system using Buildroot.

However, directly modifying the files in the output/build/\<package\>-\<version\> directory is not advisable. This is due to the fact that this directory is entirely deleted when the 'make clean' command is executed.

For this particular scenario, Buildroot offers a specialized feature known as the \<pkg\>_OVERRIDE_SRCDIR mechanism. This function involves Buildroot reading an override file. This file enables users to specify alternative locations for the source code of certain packages, directing Buildroot to use these specified locations instead of the default ones. Typically, the override file is located at (CONFIG_DIR)/local.mk. The (CONFIG_DIR) represents the directory where the Buildroot .config file is located. 
In this override file, Buildroot expects to find lines of the form:

```
<pkg1>_OVERRIDE_SRCDIR = /path/to/pkg1/sources
<pkg2>_OVERRIDE_SRCDIR = /path/to/pkg2/sources
```

In the case of Infix, **tcpdump** will be used to demonstrate the scenario.
The contents of the **local.mk** should be as follows:
```
TCPDUMP_OVERRIDE_SRCDIR = /path/to/tcpdump/repo
```
The file should be stored in the **output/** folder, alongside the **.config file**, as mentioned earlier in the text.


When Buildroot detects that a specific package has an associated \<pkg\>_OVERRIDE_SRCDIR defined, instead of attempting to download, extract, and patch the package, it will directly utilize the source code from the specified directory. Importantly, this directory remains unaffected by the 'make clean' command, ensuring its contents are preserved. This feature allows users to direct Buildroot towards their own directories, which can be managed using Git, Subversion, or other version control systems. To facilitate this, Buildroot employs **rsync** to transfer the component's source code from the \<pkg\>_OVERRIDE_SRCDIR to a new directory, output/build/**\<package\>-custom/**.


Executing 'make \<pkg\>-rebuild all' triggers a sequence where rsync syncs the source code from \<pkg\>_OVERRIDE_SRCDIR to output/build/\<package\>-custom. Subsequently, it restarts the build process for that specific package.


```
make tcpdump-rebuild all
```

By adding "all" the build process gets performed for the entire project (all other components of the Buildroot system).
As a result, the **tcpdump-custom** should show up beneath the **output/build/** repository.

```
user@PC:~/infix/output/build$ ll | grep tcpdump
drwxr-xr-x   7 group user 20480 Nov 10 18:26 tcpdump-4.99.4/
drwxr-xr-x   7 group user 12288 Nov 10 18:28 tcpdump-custom/
```

