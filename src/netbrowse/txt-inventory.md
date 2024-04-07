mDNS Browsing Inventory
=======================

Results of investigation into mDNS TXT records being used by different vendors.
For printers Apple published the Bonjour Printing Specification:

<https://developer.apple.com/bonjour/printing-specification/bonjourprinting-1.2.1.pdf>

See the Legend below for an overview of collected (known) TXT records of relevance.


Other Links
-----------

 - <https://grouper.ieee.org/groups/1722/contributions/2009/Bonjour%20Device%20Discovery.pdf>
 - <https://wiki.debian.org/CUPSAirPrint>

Available Service Types
-----------------------

<http://www.dns-sd.org/ServiceTypes.html>


Proposed TXT Records
--------------------

**Alt. 1**

        "vv=1" "product=Qemu" "ty=VM" "on=Infix" "ov=v24.03.0" "vn=KernelKit"

**Alt. 2**

        "vv=1" "vendor=Qemu" "product=VM" "ty=x86-64" "vn=KernelKit" "on=Infix" "ov=v24.03.0" 

Legend
------

Notice the difference between software and hardware/product related records.

    am : Apple Model
    mn : Model Name or role
    ty : Type
    on : OS Name
    ov : OS Version
    vn : OS Vendor Name
    vs : (Software or) firmware version
    vv : Vendor format of mDNS TXT records
    pk : Public Key, digest or unique identifier
    sf : Status Flags, detailing settings or states of the device
    tv : Version of (software or) protocol

    adminurl: Administration interface, e.g. adminurl=http://printer.local/#configPage
	apiurl  : Endpoint for REST API
	vendor  : Product Vendor name
	product : Product name
	model   : Product model
	path    : Path to service, e.g. path=/printer
	deviceid: MAC Address
	btaddr  : Bluetooth address


Home LAN
--------

```
$ avahi-browse -tarpk
+;eth2;IPv4;GIMLI;_smb._tcp;local
+;qtap9;IPv6;GIMLI;_smb._tcp;local
+;qtap8;IPv6;GIMLI;_smb._tcp;local
+;qtap7;IPv6;GIMLI;_smb._tcp;local
+;qtap6;IPv6;GIMLI;_smb._tcp;local
+;qtap5;IPv6;GIMLI;_smb._tcp;local
+;qtap4;IPv6;GIMLI;_smb._tcp;local
+;qtap3;IPv6;GIMLI;_smb._tcp;local
+;qtap2;IPv6;GIMLI;_smb._tcp;local
+;qtap1;IPv6;GIMLI;_smb._tcp;local
+;qtap0;IPv6;GIMLI;_smb._tcp;local
+;br0;IPv6;GIMLI;_smb._tcp;local
+;lxcbr0;IPv4;GIMLI;_smb._tcp;local
+;virbr0;IPv6;GIMLI;_smb._tcp;local
+;virbr0;IPv4;GIMLI;_smb._tcp;local
+;wlan0;IPv6;LUTHIEN;_smb._tcp;local
+;wlan0;IPv6;GIMLI;_smb._tcp;local
+;wlan0;IPv4;readynas\032\040CIFS\041;_smb._tcp;local
+;wlan0;IPv4;LUTHIEN;_smb._tcp;local
+;wlan0;IPv4;GIMLI;_smb._tcp;local
+;wlan0;IPv4;LIBREELEC;_smb._tcp;local
+;eth0;IPv6;LUTHIEN;_smb._tcp;local
+;eth0;IPv6;GIMLI;_smb._tcp;local
+;eth0;IPv4;readynas\032\040CIFS\041;_smb._tcp;local
+;eth0;IPv4;LUTHIEN;_smb._tcp;local
+;eth0;IPv4;LIBREELEC;_smb._tcp;local
+;eth0;IPv4;GIMLI;_smb._tcp;local
+;lo;IPv4;GIMLI;_smb._tcp;local
+;eth2;IPv4;GIMLI;_device-info._tcp;local
+;qtap9;IPv6;GIMLI;_device-info._tcp;local
+;qtap8;IPv6;GIMLI;_device-info._tcp;local
+;qtap7;IPv6;GIMLI;_device-info._tcp;local
+;qtap6;IPv6;GIMLI;_device-info._tcp;local
+;qtap5;IPv6;GIMLI;_device-info._tcp;local
+;qtap4;IPv6;GIMLI;_device-info._tcp;local
+;qtap3;IPv6;GIMLI;_device-info._tcp;local
+;qtap2;IPv6;GIMLI;_device-info._tcp;local
+;qtap1;IPv6;GIMLI;_device-info._tcp;local
+;qtap0;IPv6;GIMLI;_device-info._tcp;local
+;br0;IPv6;GIMLI;_device-info._tcp;local
+;lxcbr0;IPv4;GIMLI;_device-info._tcp;local
+;virbr0;IPv6;GIMLI;_device-info._tcp;local
+;virbr0;IPv4;GIMLI;_device-info._tcp;local
+;wlan0;IPv6;LUTHIEN;_device-info._tcp;local
+;wlan0;IPv6;GIMLI;_device-info._tcp;local
+;wlan0;IPv4;LIBREELEC;_device-info._tcp;local
+;wlan0;IPv4;LUTHIEN;_device-info._tcp;local
+;wlan0;IPv4;GIMLI;_device-info._tcp;local
+;eth0;IPv6;LUTHIEN;_device-info._tcp;local
+;eth0;IPv6;GIMLI;_device-info._tcp;local
+;eth0;IPv4;LIBREELEC;_device-info._tcp;local
+;eth0;IPv4;LUTHIEN;_device-info._tcp;local
+;eth0;IPv4;GIMLI;_device-info._tcp;local
+;lo;IPv4;GIMLI;_device-info._tcp;local
+;qtap9;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local
+;qtap8;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local
+;qtap7;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local
+;qtap6;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local
+;qtap5;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local
+;qtap4;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local
+;qtap3;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local
+;qtap2;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local
+;qtap1;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local
+;qtap0;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local
+;qtap9;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local
+;qtap8;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local
+;qtap7;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local
+;qtap6;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local
+;qtap5;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local
+;qtap4;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local
+;qtap3;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local
+;qtap2;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local
+;qtap1;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local
+;qtap0;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local
+;wlan0;IPv4;LibreELEC;_sftp-ssh._tcp;local
+;eth0;IPv4;LibreELEC;_sftp-ssh._tcp;local
+;qtap9;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local
+;qtap8;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local
+;qtap7;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local
+;qtap6;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local
+;qtap5;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local
+;qtap4;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local
+;qtap3;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local
+;qtap2;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local
+;qtap1;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local
+;qtap0;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local
+;qtap9;IPv6;Web\032Management\032Interface;_https._tcp;local
+;qtap9;IPv6;Web\032Console\032Interface;_https._tcp;local
+;qtap8;IPv6;Web\032Management\032Interface;_https._tcp;local
+;qtap8;IPv6;Web\032Console\032Interface;_https._tcp;local
+;qtap7;IPv6;Web\032Management\032Interface;_https._tcp;local
+;qtap7;IPv6;Web\032Console\032Interface;_https._tcp;local
+;qtap6;IPv6;Web\032Management\032Interface;_https._tcp;local
+;qtap6;IPv6;Web\032Console\032Interface;_https._tcp;local
+;qtap5;IPv6;Web\032Console\032Interface;_https._tcp;local
+;qtap5;IPv6;Web\032Management\032Interface;_https._tcp;local
+;qtap4;IPv6;Web\032Console\032Interface;_https._tcp;local
+;qtap4;IPv6;Web\032Management\032Interface;_https._tcp;local
+;qtap3;IPv6;Web\032Console\032Interface;_https._tcp;local
+;qtap3;IPv6;Web\032Management\032Interface;_https._tcp;local
+;qtap2;IPv6;Web\032Console\032Interface;_https._tcp;local
+;qtap2;IPv6;Web\032Management\032Interface;_https._tcp;local
+;qtap1;IPv6;Web\032Management\032Interface;_https._tcp;local
+;qtap1;IPv6;Web\032Console\032Interface;_https._tcp;local
+;qtap0;IPv6;Web\032Management\032Interface;_https._tcp;local
+;qtap0;IPv6;Web\032Console\032Interface;_https._tcp;local
+;qtap9;IPv6;Web\032Management\032Interface;_http._tcp;local
+;qtap8;IPv6;Web\032Management\032Interface;_http._tcp;local
+;qtap7;IPv6;Web\032Management\032Interface;_http._tcp;local
+;qtap6;IPv6;Web\032Management\032Interface;_http._tcp;local
+;qtap5;IPv6;Web\032Management\032Interface;_http._tcp;local
+;qtap4;IPv6;Web\032Management\032Interface;_http._tcp;local
+;qtap3;IPv6;Web\032Management\032Interface;_http._tcp;local
+;qtap2;IPv6;Web\032Management\032Interface;_http._tcp;local
+;qtap1;IPv6;Web\032Management\032Interface;_http._tcp;local
+;qtap0;IPv6;Web\032Management\032Interface;_http._tcp;local
+;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http._tcp;local
+;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http._tcp;local
+;wlan0;IPv4;Itunes\032Server\032on\032readynas;_http._tcp;local
+;wlan0;IPv4;FrontView\032on\032readynas;_http._tcp;local
+;wlan0;IPv4;Kodi\032\040LibreELEC\041;_http._tcp;local
+;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http._tcp;local
+;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http._tcp;local
+;eth0;IPv4;Itunes\032Server\032on\032readynas;_http._tcp;local
+;eth0;IPv4;FrontView\032on\032readynas;_http._tcp;local
+;eth0;IPv4;Kodi\032\040LibreELEC\041;_http._tcp;local
+;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_printer._tcp;local
+;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_printer._tcp;local
+;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_printer._tcp;local
+;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_printer._tcp;local
+;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_pdl-datastream._tcp;local
+;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_pdl-datastream._tcp;local
+;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_pdl-datastream._tcp;local
+;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_pdl-datastream._tcp;local
+;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipps._tcp;local
+;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipps._tcp;local
+;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipps._tcp;local
+;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipps._tcp;local
+;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipp._tcp;local
+;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipp._tcp;local
+;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipp._tcp;local
+;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipp._tcp;local
+;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_print-caps._tcp;local
+;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_print-caps._tcp;local
+;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_print-caps._tcp;local
+;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_print-caps._tcp;local
+;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http-alt._tcp;local
+;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http-alt._tcp;local
+;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http-alt._tcp;local
+;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http-alt._tcp;local
+;wlan0;IPv6;Apple\032BorderRouter\032\035D9D8;_meshcop._udp;local
+;wlan0;IPv4;Apple\032BorderRouter\032\035D9D8;_meshcop._udp;local
+;eth0;IPv6;Apple\032BorderRouter\032\035D9D8;_meshcop._udp;local
+;eth0;IPv4;Apple\032BorderRouter\032\035D9D8;_meshcop._udp;local
+;wlan0;IPv6;3e88005fdb7bd9d8;_trel._udp;local
+;wlan0;IPv4;3e88005fdb7bd9d8;_trel._udp;local
+;eth0;IPv6;3e88005fdb7bd9d8;_trel._udp;local
+;eth0;IPv4;3e88005fdb7bd9d8;_trel._udp;local
+;wlan0;IPv6;Living\032Room;_srpl-tls._tcp;local
+;wlan0;IPv4;Living\032Room;_srpl-tls._tcp;local
+;eth0;IPv6;Living\032Room;_srpl-tls._tcp;local
+;eth0;IPv4;Living\032Room;_srpl-tls._tcp;local
+;wlan0;IPv6;1CB3C90F49FD\064Living\032Room;_raop._tcp;local
+;wlan0;IPv4;1CB3C90F49FD\064Living\032Room;_raop._tcp;local
+;wlan0;IPv4;B827EBAE1BA7\064Kodi\032\040LibreELEC\041;_raop._tcp;local
+;eth0;IPv6;1CB3C90F49FD\064Living\032Room;_raop._tcp;local
+;eth0;IPv4;1CB3C90F49FD\064Living\032Room;_raop._tcp;local
+;eth0;IPv4;B827EBAE1BA7\064Kodi\032\040LibreELEC\041;_raop._tcp;local
+;wlan0;IPv6;Living\032Room;_airplay._tcp;local
+;wlan0;IPv4;Living\032Room;_airplay._tcp;local
+;eth0;IPv6;Living\032Room;_airplay._tcp;local
+;eth0;IPv4;Living\032Room;_airplay._tcp;local
+;wlan0;IPv6;Living\032Room;_companion-link._tcp;local
+;wlan0;IPv4;Living\032Room;_companion-link._tcp;local
+;eth0;IPv6;Living\032Room;_companion-link._tcp;local
+;eth0;IPv4;Living\032Room;_companion-link._tcp;local
+;wlan0;IPv6;70-35-60-63\.1\032Living\032Room;_sleep-proxy._udp;local
+;wlan0;IPv4;70-35-60-63\.1\032Living\032Room;_sleep-proxy._udp;local
+;eth0;IPv6;70-35-60-63\.1\032Living\032Room;_sleep-proxy._udp;local
+;eth0;IPv4;70-35-60-63\.1\032Living\032Room;_sleep-proxy._udp;local
+;wlan0;IPv4;ReadyNAS\032Discovery\032\091readynas\093;_readynas._tcp;local
+;eth0;IPv4;ReadyNAS\032Discovery\032\091readynas\093;_readynas._tcp;local
+;wlan0;IPv4;Itunes\032Server\032on\032readynas;_rsp._tcp;local
+;eth0;IPv4;Itunes\032Server\032on\032readynas;_rsp._tcp;local
+;wlan0;IPv4;Itunes\032Server\032on\032readynas;_daap._tcp;local
+;eth0;IPv4;Itunes\032Server\032on\032readynas;_daap._tcp;local
+;wlan0;IPv4;firefly\032\09174\058da\05838\0586e\0585e\0582a\093;_workstation._tcp;local
+;wlan0;IPv4;readynas\032\091a0\05821\058b7\058c1\0588c\0583a\093;_workstation._tcp;local
+;eth0;IPv4;firefly\032\09174\058da\05838\0586e\0585e\0582a\093;_workstation._tcp;local
+;eth0;IPv4;readynas\032\091a0\05821\058b7\058c1\0588c\0583a\093;_workstation._tcp;local
+;wlan0;IPv4;root\064LibreELEC;_pulse-server._tcp;local
+;eth0;IPv4;root\064LibreELEC;_pulse-server._tcp;local
+;wlan0;IPv4;root\064LibreELEC\058\032Dummy\032Output;_pulse-sink._tcp;local
+;eth0;IPv4;root\064LibreELEC\058\032Dummy\032Output;_pulse-sink._tcp;local
+;wlan0;IPv4;Kodi\032\040LibreELEC\041;_xbmc-jsonrpc-h._tcp;local
+;eth0;IPv4;Kodi\032\040LibreELEC\041;_xbmc-jsonrpc-h._tcp;local
+;wlan0;IPv4;Kodi\032\040LibreELEC\041;_xbmc-jsonrpc._tcp;local
+;eth0;IPv4;Kodi\032\040LibreELEC\041;_xbmc-jsonrpc._tcp;local
+;wlan0;IPv4;Kodi\032\040LibreELEC\041;_xbmc-events._udp;local
+;eth0;IPv4;Kodi\032\040LibreELEC\041;_xbmc-events._udp;local
=;eth0;IPv4;readynas\032\040CIFS\041;_smb._tcp;local;readynas.local;192.168.1.173;445;
=;eth0;IPv4;LIBREELEC;_smb._tcp;local;LibreELEC.local;192.168.1.227;445;
=;qtap9;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::d45b:7aff:febb:d70b;445;
=;qtap8;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::60c4:7cff:fe35:7db1;445;
=;qtap7;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::4808:95ff:fee8:18df;445;
=;qtap6;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::58b2:55ff:fef6:afe7;445;
=;qtap5;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::707d:c6ff:fed8:9764;445;
=;qtap4;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::e861:b6ff:fe68:660;445;
=;qtap3;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::6c2c:b6ff:febf:fbc5;445;
=;qtap2;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::75:b3ff:fe11:4083;445;
=;qtap1;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::a0be:ccff:fec2:fb34;445;
=;qtap0;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::b4ca:54ff:fe5e:e0b4;445;
=;br0;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::e018:99ff:fe36:51c7;445;
=;virbr0;IPv6;GIMLI;_smb._tcp;local;gimli.local;2001:db8::1;445;
=;wlan0;IPv6;GIMLI;_smb._tcp;local;gimli.local;2001:9b0:214:3500::522;445;
=;eth0;IPv6;GIMLI;_smb._tcp;local;gimli.local;2001:9b0:214:3500::522;445;
=;eth0;IPv6;LUTHIEN;_smb._tcp;local;luthien.local;2001:9b0:214:3500::c2e;445;
=;wlan0;IPv6;LUTHIEN;_smb._tcp;local;luthien.local;2001:9b0:214:3500::c2e;445;
=;qtap8;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:8;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap7;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:7;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap6;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:6;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap5;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:5;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap4;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:4;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap3;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:3;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;eth0;IPv6;LUTHIEN;_device-info._tcp;local;luthien.local;2001:9b0:214:3500::650;0;"model=MacSamba"
=;wlan0;IPv6;LUTHIEN;_device-info._tcp;local;luthien.local;2001:9b0:214:3500::650;0;"model=MacSamba"
=;qtap9;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::d45b:7aff:febb:d70b;0;"model=MacSamba"
=;qtap8;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::60c4:7cff:fe35:7db1;0;"model=MacSamba"
=;qtap7;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::4808:95ff:fee8:18df;0;"model=MacSamba"
=;qtap6;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::58b2:55ff:fef6:afe7;0;"model=MacSamba"
=;qtap5;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::707d:c6ff:fed8:9764;0;"model=MacSamba"
=;qtap4;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::e861:b6ff:fe68:660;0;"model=MacSamba"
=;qtap3;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::6c2c:b6ff:febf:fbc5;0;"model=MacSamba"
=;qtap2;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::75:b3ff:fe11:4083;0;"model=MacSamba"
=;qtap1;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::a0be:ccff:fec2:fb34;0;"model=MacSamba"
=;qtap0;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::b4ca:54ff:fe5e:e0b4;0;"model=MacSamba"
=;br0;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::e018:99ff:fe36:51c7;0;"model=MacSamba"
=;virbr0;IPv6;GIMLI;_device-info._tcp;local;gimli.local;2001:db8::1;0;"model=MacSamba"
=;wlan0;IPv6;GIMLI;_device-info._tcp;local;gimli.local;2001:9b0:214:3500::e19;0;"model=MacSamba"
=;eth0;IPv6;GIMLI;_device-info._tcp;local;gimli.local;2001:9b0:214:3500::e19;0;"model=MacSamba"
=;eth0;IPv4;LUTHIEN;_smb._tcp;local;luthien.local;192.168.1.234;445;
=;eth2;IPv4;GIMLI;_smb._tcp;local;gimli.local;192.168.2.1;445;
=;lxcbr0;IPv4;GIMLI;_smb._tcp;local;gimli.local;10.0.3.1;445;
=;virbr0;IPv4;GIMLI;_smb._tcp;local;gimli.local;192.168.122.1;445;
=;wlan0;IPv4;GIMLI;_smb._tcp;local;gimli.local;192.168.1.236;445;
=;wlan0;IPv4;readynas\032\040CIFS\041;_smb._tcp;local;readynas.local;192.168.1.173;445;
=;wlan0;IPv4;LIBREELEC;_smb._tcp;local;LibreELEC.local;192.168.1.227;445;
=;wlan0;IPv4;LUTHIEN;_smb._tcp;local;luthien.local;192.168.1.234;445;
=;eth0;IPv4;LUTHIEN;_device-info._tcp;local;luthien.local;192.168.1.132;0;"model=MacSamba"
=;eth0;IPv4;LIBREELEC;_device-info._tcp;local;LibreELEC.local;192.168.1.227;0;"model=Xserve"
=;wlan0;IPv4;LUTHIEN;_device-info._tcp;local;luthien.local;192.168.1.132;0;"model=MacSamba"
=;qtap9;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:9;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;wlan0;IPv4;LIBREELEC;_device-info._tcp;local;LibreELEC.local;192.168.1.227;0;"model=Xserve"
=;eth0;IPv4;GIMLI;_smb._tcp;local;gimli.local;192.168.1.131;445;
=;eth0;IPv4;GIMLI;_device-info._tcp;local;gimli.local;192.168.1.131;0;"model=MacSamba"
=;lo;IPv4;GIMLI;_device-info._tcp;local;gimli.local;127.0.0.1;0;"model=MacSamba"
=;lo;IPv4;GIMLI;_smb._tcp;local;gimli.local;127.0.0.1;445;
=;eth2;IPv4;GIMLI;_device-info._tcp;local;gimli.local;192.168.2.1;0;"model=MacSamba"
=;lxcbr0;IPv4;GIMLI;_device-info._tcp;local;gimli.local;10.0.3.1;0;"model=MacSamba"
=;virbr0;IPv4;GIMLI;_device-info._tcp;local;gimli.local;192.168.122.1;0;"model=MacSamba"
=;wlan0;IPv4;GIMLI;_device-info._tcp;local;gimli.local;192.168.1.236;0;"model=MacSamba"
=;qtap2;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:2;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap2;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:2;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap2;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:2;830;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap1;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:1;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap1;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:1;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap1;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:1;830;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap0;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:0;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap0;IPv6;Secure\032shell\032command\032line\032interface\032\040CLI\041;_ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:0;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap0;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:0;830;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap9;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:9;830;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap9;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:9;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap9;IPv6;Web\032Console\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:9;443;"adminurl=https//infix-00-00-00.local/console" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap9;IPv6;Web\032Management\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:9;443;"adminurl=https://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap8;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:8;830;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap8;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:8;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap8;IPv6;Web\032Console\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:8;443;"adminurl=https//infix-00-00-00.local/console" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap8;IPv6;Web\032Management\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:8;443;"adminurl=https://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap7;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:7;830;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap7;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:7;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap7;IPv6;Web\032Console\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:7;443;"adminurl=https//infix-00-00-00.local/console" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap7;IPv6;Web\032Management\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:7;443;"adminurl=https://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap6;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:6;830;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap6;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:6;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap6;IPv6;Web\032Console\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:6;443;"adminurl=https//infix-00-00-00.local/console" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap6;IPv6;Web\032Management\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:6;443;"adminurl=https://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap5;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:5;830;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap5;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:5;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap5;IPv6;Web\032Management\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:5;443;"adminurl=https://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap5;IPv6;Web\032Console\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:5;443;"adminurl=https//infix-00-00-00.local/console" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap4;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:4;830;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap4;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:4;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap4;IPv6;Web\032Management\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:4;443;"adminurl=https://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap4;IPv6;Web\032Console\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:4;443;"adminurl=https//infix-00-00-00.local/console" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap3;IPv6;NETCONF\032\040XML\047SSH\041;_netconf-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:3;830;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap3;IPv6;Secure\032file\032transfer\032\040FTP\047SSH\041;_sftp-ssh._tcp;local;infix-00-00-00.local;fe80::ff:fe00:3;22;"product=Infix v24.02.0-96-gc1770c40-dirty"
=;eth0;IPv4;LibreELEC;_sftp-ssh._tcp;local;LibreELEC.local;192.168.1.227;22;
=;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;80;"UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http._tcp;local;NPIC9167E.local;192.168.1.172;80;"UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;qtap2;IPv6;Web\032Console\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:2;443;"adminurl=https//infix-00-00-00.local/console" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap2;IPv6;Web\032Management\032Interface;_http._tcp;local;infix-00-00-00.local;fe80::ff:fe00:2;80;"adminurl=http://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap2;IPv6;Web\032Management\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:2;443;"adminurl=https://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap1;IPv6;Web\032Management\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:1;443;"adminurl=https://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap1;IPv6;Web\032Management\032Interface;_http._tcp;local;infix-00-00-00.local;fe80::ff:fe00:1;80;"adminurl=http://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap1;IPv6;Web\032Console\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:1;443;"adminurl=https//infix-00-00-00.local/console" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap0;IPv6;Web\032Management\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:0;443;"adminurl=https://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap0;IPv6;Web\032Management\032Interface;_http._tcp;local;infix-00-00-00.local;fe80::ff:fe00:0;80;"adminurl=http://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap0;IPv6;Web\032Console\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:0;443;"adminurl=https//infix-00-00-00.local/console" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap3;IPv6;Web\032Management\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:3;443;"adminurl=https://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap3;IPv6;Web\032Console\032Interface;_https._tcp;local;infix-00-00-00.local;fe80::ff:fe00:3;443;"adminurl=https//infix-00-00-00.local/console" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap3;IPv6;Web\032Management\032Interface;_http._tcp;local;infix-00-00-00.local;fe80::ff:fe00:3;80;"adminurl=http://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap9;IPv6;Web\032Management\032Interface;_http._tcp;local;infix-00-00-00.local;fe80::ff:fe00:9;80;"adminurl=http://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap8;IPv6;Web\032Management\032Interface;_http._tcp;local;infix-00-00-00.local;fe80::ff:fe00:8;80;"adminurl=http://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap7;IPv6;Web\032Management\032Interface;_http._tcp;local;infix-00-00-00.local;fe80::ff:fe00:7;80;"adminurl=http://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap6;IPv6;Web\032Management\032Interface;_http._tcp;local;infix-00-00-00.local;fe80::ff:fe00:6;80;"adminurl=http://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap5;IPv6;Web\032Management\032Interface;_http._tcp;local;infix-00-00-00.local;fe80::ff:fe00:5;80;"adminurl=http://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;qtap4;IPv6;Web\032Management\032Interface;_http._tcp;local;infix-00-00-00.local;fe80::ff:fe00:4;80;"adminurl=http://infix-00-00-00.local" "product=Infix v24.02.0-96-gc1770c40-dirty"
=;wlan0;IPv4;LibreELEC;_sftp-ssh._tcp;local;LibreELEC.local;192.168.1.227;22;
=;wlan0;IPv4;Itunes\032Server\032on\032readynas;_http._tcp;local;readynas.local;192.168.1.173;3689;"ffid=7e725542" "Password=false" "Version=196610" "iTSh Version=131073" "mtd-version=svn-1676" "Machine Name=Itunes Server" "Machine ID=6C37BA14" "Database ID=6C37BA14" "txtvers=1"
=;wlan0;IPv4;FrontView\032on\032readynas;_http._tcp;local;readynas.local;192.168.1.173;80;"path=/admin/"
=;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;80;"UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http._tcp;local;NPIC9167E.local;192.168.1.172;80;"UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;eth0;IPv4;Itunes\032Server\032on\032readynas;_http._tcp;local;readynas.local;192.168.1.173;3689;"ffid=7e725542" "Password=false" "Version=196610" "iTSh Version=131073" "mtd-version=svn-1676" "Machine Name=Itunes Server" "Machine ID=6C37BA14" "Database ID=6C37BA14" "txtvers=1"
=;eth0;IPv4;FrontView\032on\032readynas;_http._tcp;local;readynas.local;192.168.1.173;80;"path=/admin/"
=;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_printer._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;515;"TBCP=T" "Binary=T" "Transparent=T" "note=" "adminurl=http://NPIC9167E.local." "priority=50" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "rp=RAW" "qtotal=1" "txtvers=1" "UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_printer._tcp;local;NPIC9167E.local;192.168.1.172;515;"TBCP=T" "Binary=T" "Transparent=T" "note=" "adminurl=http://NPIC9167E.local." "priority=50" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "rp=RAW" "qtotal=1" "txtvers=1" "UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_printer._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;515;"TBCP=T" "Binary=T" "Transparent=T" "note=" "adminurl=http://NPIC9167E.local." "priority=50" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "rp=RAW" "qtotal=1" "txtvers=1" "UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_printer._tcp;local;NPIC9167E.local;192.168.1.172;515;"TBCP=T" "Binary=T" "Transparent=T" "note=" "adminurl=http://NPIC9167E.local." "priority=50" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "rp=RAW" "qtotal=1" "txtvers=1" "UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_pdl-datastream._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;9100;"TBCP=T" "Binary=T" "Transparent=T" "note=" "adminurl=http://NPIC9167E.local." "priority=40" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "qtotal=1" "txtvers=1" "UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_pdl-datastream._tcp;local;NPIC9167E.local;192.168.1.172;9100;"TBCP=T" "Binary=T" "Transparent=T" "note=" "adminurl=http://NPIC9167E.local." "priority=40" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "qtotal=1" "txtvers=1" "UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_pdl-datastream._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;9100;"TBCP=T" "Binary=T" "Transparent=T" "note=" "adminurl=http://NPIC9167E.local." "priority=40" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "qtotal=1" "txtvers=1" "UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_pdl-datastream._tcp;local;NPIC9167E.local;192.168.1.172;9100;"TBCP=T" "Binary=T" "Transparent=T" "note=" "adminurl=http://NPIC9167E.local." "priority=40" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "qtotal=1" "txtvers=1" "UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipps._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;443;"mopria_certified=1.2" "mac=48:0f:cf:c9:16:7e" "usb_MDL=HP Color LaserJet Pro M252dw" "usb_MFG=Hewlett-Packard" "TLS=1.2" "PaperMax=legal-A4" "kind=document,envelope,photo" "UUID=564e4333-4e30-3439-3539-480fcfc9167e" "Fax=F" "Scan=F" "Duplex=T" "Color=T" "note=" "adminurl=https://NPIC9167E.local./hp/device/info_config_AirPrint.html?tab=Networking&menu=AirPrintStatus" "priority=10" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "URF=V1.4,CP99,W8,OB10,PQ3-4-5,ADOBERGB24,DEVRGB24,DEVW8,SRGB24,DM1,IS1,MT1-2-3-5-12,RS600" "rp=ipp/print" "pdl=image/urf,application/pdf,application/postscript,application/vnd.hp-PCL,application/vnd.hp-PCLXL,application/PCLm,application/octet-stream,image/jpeg" "qtotal=1" "txtvers=1"
=;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipps._tcp;local;NPIC9167E.local;192.168.1.172;443;"mopria_certified=1.2" "mac=48:0f:cf:c9:16:7e" "usb_MDL=HP Color LaserJet Pro M252dw" "usb_MFG=Hewlett-Packard" "TLS=1.2" "PaperMax=legal-A4" "kind=document,envelope,photo" "UUID=564e4333-4e30-3439-3539-480fcfc9167e" "Fax=F" "Scan=F" "Duplex=T" "Color=T" "note=" "adminurl=https://NPIC9167E.local./hp/device/info_config_AirPrint.html?tab=Networking&menu=AirPrintStatus" "priority=10" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "URF=V1.4,CP99,W8,OB10,PQ3-4-5,ADOBERGB24,DEVRGB24,DEVW8,SRGB24,DM1,IS1,MT1-2-3-5-12,RS600" "rp=ipp/print" "pdl=image/urf,application/pdf,application/postscript,application/vnd.hp-PCL,application/vnd.hp-PCLXL,application/PCLm,application/octet-stream,image/jpeg" "qtotal=1" "txtvers=1"
=;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipps._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;443;"mopria_certified=1.2" "mac=48:0f:cf:c9:16:7e" "usb_MDL=HP Color LaserJet Pro M252dw" "usb_MFG=Hewlett-Packard" "TLS=1.2" "PaperMax=legal-A4" "kind=document,envelope,photo" "UUID=564e4333-4e30-3439-3539-480fcfc9167e" "Fax=F" "Scan=F" "Duplex=T" "Color=T" "note=" "adminurl=https://NPIC9167E.local./hp/device/info_config_AirPrint.html?tab=Networking&menu=AirPrintStatus" "priority=10" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "URF=V1.4,CP99,W8,OB10,PQ3-4-5,ADOBERGB24,DEVRGB24,DEVW8,SRGB24,DM1,IS1,MT1-2-3-5-12,RS600" "rp=ipp/print" "pdl=image/urf,application/pdf,application/postscript,application/vnd.hp-PCL,application/vnd.hp-PCLXL,application/PCLm,application/octet-stream,image/jpeg" "qtotal=1" "txtvers=1"
=;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipps._tcp;local;NPIC9167E.local;192.168.1.172;443;"mopria_certified=1.2" "mac=48:0f:cf:c9:16:7e" "usb_MDL=HP Color LaserJet Pro M252dw" "usb_MFG=Hewlett-Packard" "TLS=1.2" "PaperMax=legal-A4" "kind=document,envelope,photo" "UUID=564e4333-4e30-3439-3539-480fcfc9167e" "Fax=F" "Scan=F" "Duplex=T" "Color=T" "note=" "adminurl=https://NPIC9167E.local./hp/device/info_config_AirPrint.html?tab=Networking&menu=AirPrintStatus" "priority=10" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "URF=V1.4,CP99,W8,OB10,PQ3-4-5,ADOBERGB24,DEVRGB24,DEVW8,SRGB24,DM1,IS1,MT1-2-3-5-12,RS600" "rp=ipp/print" "pdl=image/urf,application/pdf,application/postscript,application/vnd.hp-PCL,application/vnd.hp-PCLXL,application/PCLm,application/octet-stream,image/jpeg" "qtotal=1" "txtvers=1"
=;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipp._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;631;"mopria_certified=1.2" "mac=48:0f:cf:c9:16:7e" "usb_MDL=HP Color LaserJet Pro M252dw" "usb_MFG=Hewlett-Packard" "TLS=1.2" "PaperMax=legal-A4" "kind=document,envelope,photo" "UUID=564e4333-4e30-3439-3539-480fcfc9167e" "Fax=F" "Scan=F" "Duplex=T" "Color=T" "note=" "adminurl=http://NPIC9167E.local./hp/device/info_config_AirPrint.html?tab=Networking&menu=AirPrintStatus" "priority=10" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "URF=V1.4,CP99,W8,OB10,PQ3-4-5,ADOBERGB24,DEVRGB24,DEVW8,SRGB24,DM1,IS1,MT1-2-3-5-12,RS600" "rp=ipp/print" "pdl=image/urf,application/pdf,application/postscript,application/vnd.hp-PCL,application/vnd.hp-PCLXL,application/PCLm,application/octet-stream,image/jpeg" "qtotal=1" "txtvers=1"
=;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipp._tcp;local;NPIC9167E.local;192.168.1.172;631;"mopria_certified=1.2" "mac=48:0f:cf:c9:16:7e" "usb_MDL=HP Color LaserJet Pro M252dw" "usb_MFG=Hewlett-Packard" "TLS=1.2" "PaperMax=legal-A4" "kind=document,envelope,photo" "UUID=564e4333-4e30-3439-3539-480fcfc9167e" "Fax=F" "Scan=F" "Duplex=T" "Color=T" "note=" "adminurl=http://NPIC9167E.local./hp/device/info_config_AirPrint.html?tab=Networking&menu=AirPrintStatus" "priority=10" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "URF=V1.4,CP99,W8,OB10,PQ3-4-5,ADOBERGB24,DEVRGB24,DEVW8,SRGB24,DM1,IS1,MT1-2-3-5-12,RS600" "rp=ipp/print" "pdl=image/urf,application/pdf,application/postscript,application/vnd.hp-PCL,application/vnd.hp-PCLXL,application/PCLm,application/octet-stream,image/jpeg" "qtotal=1" "txtvers=1"
=;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipp._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;631;"mopria_certified=1.2" "mac=48:0f:cf:c9:16:7e" "usb_MDL=HP Color LaserJet Pro M252dw" "usb_MFG=Hewlett-Packard" "TLS=1.2" "PaperMax=legal-A4" "kind=document,envelope,photo" "UUID=564e4333-4e30-3439-3539-480fcfc9167e" "Fax=F" "Scan=F" "Duplex=T" "Color=T" "note=" "adminurl=http://NPIC9167E.local./hp/device/info_config_AirPrint.html?tab=Networking&menu=AirPrintStatus" "priority=10" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "URF=V1.4,CP99,W8,OB10,PQ3-4-5,ADOBERGB24,DEVRGB24,DEVW8,SRGB24,DM1,IS1,MT1-2-3-5-12,RS600" "rp=ipp/print" "pdl=image/urf,application/pdf,application/postscript,application/vnd.hp-PCL,application/vnd.hp-PCLXL,application/PCLm,application/octet-stream,image/jpeg" "qtotal=1" "txtvers=1"
=;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_ipp._tcp;local;NPIC9167E.local;192.168.1.172;631;"mopria_certified=1.2" "mac=48:0f:cf:c9:16:7e" "usb_MDL=HP Color LaserJet Pro M252dw" "usb_MFG=Hewlett-Packard" "TLS=1.2" "PaperMax=legal-A4" "kind=document,envelope,photo" "UUID=564e4333-4e30-3439-3539-480fcfc9167e" "Fax=F" "Scan=F" "Duplex=T" "Color=T" "note=" "adminurl=http://NPIC9167E.local./hp/device/info_config_AirPrint.html?tab=Networking&menu=AirPrintStatus" "priority=10" "product=(HP Color LaserJet Pro M252dw)" "ty=HP Color LaserJet Pro M252dw" "URF=V1.4,CP99,W8,OB10,PQ3-4-5,ADOBERGB24,DEVRGB24,DEVW8,SRGB24,DM1,IS1,MT1-2-3-5-12,RS600" "rp=ipp/print" "pdl=image/urf,application/pdf,application/postscript,application/vnd.hp-PCL,application/vnd.hp-PCLXL,application/PCLm,application/octet-stream,image/jpeg" "qtotal=1" "txtvers=1"
=;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_print-caps._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;8080;"UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_print-caps._tcp;local;NPIC9167E.local;192.168.1.172;8080;"UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_print-caps._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;8080;"UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_print-caps._tcp;local;NPIC9167E.local;192.168.1.172;8080;"UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;wlan0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http-alt._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;8080;"UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;wlan0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http-alt._tcp;local;NPIC9167E.local;192.168.1.172;8080;"UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;eth0;IPv6;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http-alt._tcp;local;NPIC9167E.local;fdd5:659a:93ce::341;8080;"UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;eth0;IPv4;HP\032Color\032LaserJet\032Pro\032M252dw\032\040C9167E\041;_http-alt._tcp;local;NPIC9167E.local;192.168.1.172;8080;"UUID=564e4333-4e30-3439-3539-480fcfc9167e"
=;wlan0;IPv6;Apple\032BorderRouter\032\035D9D8;_meshcop._udp;local;Living-Room.local;192.168.1.197;49154;"dn=DefaultDomain" "bb=\240\191" "sq=T" "pt=;7\180\009" "at=\000\000e[\145\147\000\000" "sb=\000\000\001\177" "dd=>\136\000_\219{\217\216" "xa=>\136\000_\219{\217\216" "tv=1.3.0" "xp=\194\229\194\014\243\175Jl" "nn=MyHome1433094499" "mn=BorderRouter" "vn=Apple Inc." "rv=1"
=;eth0;IPv4;Kodi\032\040LibreELEC\041;_http._tcp;local;LibreELEC.local;192.168.1.227;8080;"uuid=58c530ac-0bb0-4f99-bbc6-c9d81639c72d" "txtvers=1"
=;wlan0;IPv4;Apple\032BorderRouter\032\035D9D8;_meshcop._udp;local;Living-Room.local;192.168.1.197;49154;"dn=DefaultDomain" "bb=\240\191" "sq=T" "pt=;7\180\009" "at=\000\000e[\145\147\000\000" "sb=\000\000\001\177" "dd=>\136\000_\219{\217\216" "xa=>\136\000_\219{\217\216" "tv=1.3.0" "xp=\194\229\194\014\243\175Jl" "nn=MyHome1433094499" "mn=BorderRouter" "vn=Apple Inc." "rv=1"
=;eth0;IPv6;Apple\032BorderRouter\032\035D9D8;_meshcop._udp;local;Living-Room.local;192.168.1.197;49154;"dn=DefaultDomain" "bb=\240\191" "sq=T" "pt=;7\180\009" "at=\000\000e[\145\147\000\000" "sb=\000\000\001\177" "dd=>\136\000_\219{\217\216" "xa=>\136\000_\219{\217\216" "tv=1.3.0" "xp=\194\229\194\014\243\175Jl" "nn=MyHome1433094499" "mn=BorderRouter" "vn=Apple Inc." "rv=1"
=;eth0;IPv4;Apple\032BorderRouter\032\035D9D8;_meshcop._udp;local;Living-Room.local;192.168.1.197;49154;"dn=DefaultDomain" "bb=\240\191" "sq=T" "pt=;7\180\009" "at=\000\000e[\145\147\000\000" "sb=\000\000\001\177" "dd=>\136\000_\219{\217\216" "xa=>\136\000_\219{\217\216" "tv=1.3.0" "xp=\194\229\194\014\243\175Jl" "nn=MyHome1433094499" "mn=BorderRouter" "vn=Apple Inc." "rv=1"
=;wlan0;IPv6;3e88005fdb7bd9d8;_trel._udp;local;Living-Room.local;192.168.1.197;51426;"xp=\194\229\194\014\243\175Jl" "xa=>\136\000_\219{\217\216"
=;wlan0;IPv4;3e88005fdb7bd9d8;_trel._udp;local;Living-Room.local;192.168.1.197;51426;"xp=\194\229\194\014\243\175Jl" "xa=>\136\000_\219{\217\216"
=;eth0;IPv6;3e88005fdb7bd9d8;_trel._udp;local;Living-Room.local;192.168.1.197;51426;"xp=\194\229\194\014\243\175Jl" "xa=>\136\000_\219{\217\216"
=;eth0;IPv4;3e88005fdb7bd9d8;_trel._udp;local;Living-Room.local;192.168.1.197;51426;"xp=\194\229\194\014\243\175Jl" "xa=>\136\000_\219{\217\216"
=;wlan0;IPv6;Living\032Room;_srpl-tls._tcp;local;Living-Room.local;192.168.1.197;853;"xpanid=c2e5c20ef3af4a6c" "did=5c2101b3d88c4acf" "pid=a3c93bc433606826" "dn=openthread.thread.home.arpa."
=;wlan0;IPv4;Living\032Room;_srpl-tls._tcp;local;Living-Room.local;192.168.1.197;853;"xpanid=c2e5c20ef3af4a6c" "did=5c2101b3d88c4acf" "pid=a3c93bc433606826" "dn=openthread.thread.home.arpa."
=;eth0;IPv6;Living\032Room;_srpl-tls._tcp;local;Living-Room.local;192.168.1.197;853;"xpanid=c2e5c20ef3af4a6c" "did=5c2101b3d88c4acf" "pid=a3c93bc433606826" "dn=openthread.thread.home.arpa."
=;eth0;IPv4;Living\032Room;_srpl-tls._tcp;local;Living-Room.local;192.168.1.197;853;"xpanid=c2e5c20ef3af4a6c" "did=5c2101b3d88c4acf" "pid=a3c93bc433606826" "dn=openthread.thread.home.arpa."
=;wlan0;IPv6;1CB3C90F49FD\064Living\032Room;_raop._tcp;local;Living-Room.local;192.168.1.197;7000;"vv=1" "ov=17.4" "vs=760.20.1" "vn=65537" "tp=UDP" "pk=dcd3a159312bf6ba5369680a5642e919ef6a4b1549584e6e6f2d234fa4d6890d" "am=AppleTV11,1" "md=0,1,2" "sf=0x644" "ft=0x4A7FDFD5,0xBC177FDE" "et=0,3,5" "da=true" "cn=0,1,2,3"
=;wlan0;IPv4;1CB3C90F49FD\064Living\032Room;_raop._tcp;local;Living-Room.local;192.168.1.197;7000;"vv=1" "ov=17.4" "vs=760.20.1" "vn=65537" "tp=UDP" "pk=dcd3a159312bf6ba5369680a5642e919ef6a4b1549584e6e6f2d234fa4d6890d" "am=AppleTV11,1" "md=0,1,2" "sf=0x644" "ft=0x4A7FDFD5,0xBC177FDE" "et=0,3,5" "da=true" "cn=0,1,2,3"
=;eth0;IPv6;1CB3C90F49FD\064Living\032Room;_raop._tcp;local;Living-Room.local;192.168.1.197;7000;"vv=1" "ov=17.4" "vs=760.20.1" "vn=65537" "tp=UDP" "pk=dcd3a159312bf6ba5369680a5642e919ef6a4b1549584e6e6f2d234fa4d6890d" "am=AppleTV11,1" "md=0,1,2" "sf=0x644" "ft=0x4A7FDFD5,0xBC177FDE" "et=0,3,5" "da=true" "cn=0,1,2,3"
=;eth0;IPv4;1CB3C90F49FD\064Living\032Room;_raop._tcp;local;Living-Room.local;192.168.1.197;7000;"vv=1" "ov=17.4" "vs=760.20.1" "vn=65537" "tp=UDP" "pk=dcd3a159312bf6ba5369680a5642e919ef6a4b1549584e6e6f2d234fa4d6890d" "am=AppleTV11,1" "md=0,1,2" "sf=0x644" "ft=0x4A7FDFD5,0xBC177FDE" "et=0,3,5" "da=true" "cn=0,1,2,3"
=;wlan0;IPv6;Living\032Room;_airplay._tcp;local;Living-Room.local;192.168.1.197;7000;"vv=1" "osvers=17.4" "srcvers=760.20.1" "pk=dcd3a159312bf6ba5369680a5642e919ef6a4b1549584e6e6f2d234fa4d6890d" "psi=74EBA0AF-DB0F-483F-A9E6-0B4EB6C82857" "pi=ff37ee9c-0677-4ee4-a2c3-7cc3071c42c7" "protovers=1.1" "model=AppleTV11,1" "gcgl=1" "igl=1" "gid=8AB8C139-61B6-404C-BBE5-151E2601E356" "flags=0x644" "features=0x4A7FDFD5,0xBC177FDE" "fex=1d9/St5/F7w4oQY" "deviceid=1C:B3:C9:0F:49:FD" "btaddr=00:00:00:00:00:00" "acl=0"
=;wlan0;IPv4;Living\032Room;_airplay._tcp;local;Living-Room.local;192.168.1.197;7000;"vv=1" "osvers=17.4" "srcvers=760.20.1" "pk=dcd3a159312bf6ba5369680a5642e919ef6a4b1549584e6e6f2d234fa4d6890d" "psi=74EBA0AF-DB0F-483F-A9E6-0B4EB6C82857" "pi=ff37ee9c-0677-4ee4-a2c3-7cc3071c42c7" "protovers=1.1" "model=AppleTV11,1" "gcgl=1" "igl=1" "gid=8AB8C139-61B6-404C-BBE5-151E2601E356" "flags=0x644" "features=0x4A7FDFD5,0xBC177FDE" "fex=1d9/St5/F7w4oQY" "deviceid=1C:B3:C9:0F:49:FD" "btaddr=00:00:00:00:00:00" "acl=0"
=;eth0;IPv6;Living\032Room;_airplay._tcp;local;Living-Room.local;192.168.1.197;7000;"vv=1" "osvers=17.4" "srcvers=760.20.1" "pk=dcd3a159312bf6ba5369680a5642e919ef6a4b1549584e6e6f2d234fa4d6890d" "psi=74EBA0AF-DB0F-483F-A9E6-0B4EB6C82857" "pi=ff37ee9c-0677-4ee4-a2c3-7cc3071c42c7" "protovers=1.1" "model=AppleTV11,1" "gcgl=1" "igl=1" "gid=8AB8C139-61B6-404C-BBE5-151E2601E356" "flags=0x644" "features=0x4A7FDFD5,0xBC177FDE" "fex=1d9/St5/F7w4oQY" "deviceid=1C:B3:C9:0F:49:FD" "btaddr=00:00:00:00:00:00" "acl=0"
=;eth0;IPv4;Living\032Room;_airplay._tcp;local;Living-Room.local;192.168.1.197;7000;"vv=1" "osvers=17.4" "srcvers=760.20.1" "pk=dcd3a159312bf6ba5369680a5642e919ef6a4b1549584e6e6f2d234fa4d6890d" "psi=74EBA0AF-DB0F-483F-A9E6-0B4EB6C82857" "pi=ff37ee9c-0677-4ee4-a2c3-7cc3071c42c7" "protovers=1.1" "model=AppleTV11,1" "gcgl=1" "igl=1" "gid=8AB8C139-61B6-404C-BBE5-151E2601E356" "flags=0x644" "features=0x4A7FDFD5,0xBC177FDE" "fex=1d9/St5/F7w4oQY" "deviceid=1C:B3:C9:0F:49:FD" "btaddr=00:00:00:00:00:00" "acl=0"
=;wlan0;IPv6;Living\032Room;_companion-link._tcp;local;Living-Room.local;192.168.1.197;49153;"rpMRtID=74EBA0AF-DB0F-483F-A9E6-0B4EB6C82857" "rpBA=57:46:6D:57:B4:50" "rpHI=d3660be33890" "rpAD=404149af4259" "rpVr=543.1" "rpMd=AppleTV11,1" "rpHA=1b4700ce1a10" "rpFl=0xB67A2" "rpHN=5c9257b8eae5" "rpMac=0"
=;wlan0;IPv4;Living\032Room;_companion-link._tcp;local;Living-Room.local;192.168.1.197;49153;"rpMRtID=74EBA0AF-DB0F-483F-A9E6-0B4EB6C82857" "rpBA=57:46:6D:57:B4:50" "rpHI=d3660be33890" "rpAD=404149af4259" "rpVr=543.1" "rpMd=AppleTV11,1" "rpHA=1b4700ce1a10" "rpFl=0xB67A2" "rpHN=5c9257b8eae5" "rpMac=0"
=;wlan0;IPv4;Kodi\032\040LibreELEC\041;_http._tcp;local;LibreELEC.local;192.168.1.227;8080;"uuid=58c530ac-0bb0-4f99-bbc6-c9d81639c72d" "txtvers=1"
=;eth0;IPv6;Living\032Room;_companion-link._tcp;local;Living-Room.local;192.168.1.197;49153;"rpMRtID=74EBA0AF-DB0F-483F-A9E6-0B4EB6C82857" "rpBA=57:46:6D:57:B4:50" "rpHI=d3660be33890" "rpAD=404149af4259" "rpVr=543.1" "rpMd=AppleTV11,1" "rpHA=1b4700ce1a10" "rpFl=0xB67A2" "rpHN=5c9257b8eae5" "rpMac=0"
=;eth0;IPv4;Living\032Room;_companion-link._tcp;local;Living-Room.local;192.168.1.197;49153;"rpMRtID=74EBA0AF-DB0F-483F-A9E6-0B4EB6C82857" "rpBA=57:46:6D:57:B4:50" "rpHI=d3660be33890" "rpAD=404149af4259" "rpVr=543.1" "rpMd=AppleTV11,1" "rpHA=1b4700ce1a10" "rpFl=0xB67A2" "rpHN=5c9257b8eae5" "rpMac=0"
=;wlan0;IPv6;70-35-60-63\.1\032Living\032Room;_sleep-proxy._udp;local;Living-Room.local;192.168.1.197;50103;
=;wlan0;IPv4;70-35-60-63\.1\032Living\032Room;_sleep-proxy._udp;local;Living-Room.local;192.168.1.197;50103;
=;eth0;IPv6;70-35-60-63\.1\032Living\032Room;_sleep-proxy._udp;local;Living-Room.local;192.168.1.197;50103;
=;eth0;IPv4;70-35-60-63\.1\032Living\032Room;_sleep-proxy._udp;local;Living-Room.local;192.168.1.197;50103;
=;eth0;IPv4;ReadyNAS\032Discovery\032\091readynas\093;_readynas._tcp;local;readynas.local;192.168.1.173;9;"raid=X-RAID2" "channels=4" "serial=2DK61B0G00662" "model=ReadyNAS Ultra 4" "vendor=NETGEAR"
=;eth0;IPv4;B827EBAE1BA7\064Kodi\032\040LibreELEC\041;_raop._tcp;local;LibreELEC.local;192.168.1.227;36666;"vs=130.14" "am=Kodi,1" "md=0,1,2" "da=true" "vn=3" "pw=false" "sr=44100" "ss=16" "sm=false" "tp=UDP" "sv=false" "et=0,1" "ek=1" "ch=2" "cn=0,1" "txtvers=1"
=;wlan0;IPv4;ReadyNAS\032Discovery\032\091readynas\093;_readynas._tcp;local;readynas.local;192.168.1.173;9;"raid=X-RAID2" "channels=4" "serial=2DK61B0G00662" "model=ReadyNAS Ultra 4" "vendor=NETGEAR"
=;wlan0;IPv4;B827EBAE1BA7\064Kodi\032\040LibreELEC\041;_raop._tcp;local;LibreELEC.local;192.168.1.227;36666;"vs=130.14" "am=Kodi,1" "md=0,1,2" "da=true" "vn=3" "pw=false" "sr=44100" "ss=16" "sm=false" "tp=UDP" "sv=false" "et=0,1" "ek=1" "ch=2" "cn=0,1" "txtvers=1"
=;eth0;IPv4;Itunes\032Server\032on\032readynas;_daap._tcp;local;readynas.local;192.168.1.173;3689;"ffid=7e725542" "Password=false" "Version=196610" "iTSh Version=131073" "mtd-version=svn-1676" "Machine Name=Itunes Server" "Machine ID=6C37BA14" "Database ID=6C37BA14" "txtvers=1"
=;eth0;IPv4;Itunes\032Server\032on\032readynas;_rsp._tcp;local;readynas.local;192.168.1.173;3689;"ffid=7e725542" "Password=false" "Version=196610" "iTSh Version=131073" "mtd-version=svn-1676" "Machine Name=Itunes Server" "Machine ID=6C37BA14" "Database ID=6C37BA14" "txtvers=1"
=;eth0;IPv4;readynas\032\091a0\05821\058b7\058c1\0588c\0583a\093;_workstation._tcp;local;readynas.local;192.168.1.173;9;
=;eth0;IPv4;root\064LibreELEC;_pulse-server._tcp;local;LibreELEC.local;192.168.1.227;4713;"cookie=0xcaa87808" "fqdn=LibreELEC" "uname=Linux armv7l 4.19.127 #1 SMP Tue Jul 6 19:08:28 CEST 2021" "machine-id=0591ddc66796df8c2bb9bd455b2cd975" "user-name=root" "server-version=pulseaudio 12.2"
=;eth0;IPv4;Kodi\032\040LibreELEC\041;_xbmc-events._udp;local;LibreELEC.local;192.168.1.227;9777;
=;eth0;IPv4;Kodi\032\040LibreELEC\041;_xbmc-jsonrpc._tcp;local;LibreELEC.local;192.168.1.227;9090;"uuid=58c530ac-0bb0-4f99-bbc6-c9d81639c72d" "txtvers=1"
=;eth0;IPv4;Kodi\032\040LibreELEC\041;_xbmc-jsonrpc-h._tcp;local;LibreELEC.local;192.168.1.227;8080;"uuid=58c530ac-0bb0-4f99-bbc6-c9d81639c72d" "txtvers=1"
=;eth0;IPv4;root\064LibreELEC\058\032Dummy\032Output;_pulse-sink._tcp;local;LibreELEC.local;192.168.1.227;4713;"icon-name=computer" "class=abstract" "description=Dummy Output" "subtype=virtual" "channel_map=front-left,front-right" "format=s16le" "channels=2" "rate=44100" "device=auto_null" "cookie=0xcaa87808" "fqdn=LibreELEC" "uname=Linux armv7l 4.19.127 #1 SMP Tue Jul 6 19:08:28 CEST 2021" "machine-id=0591ddc66796df8c2bb9bd455b2cd975" "user-name=root" "server-version=pulseaudio 12.2"
=;wlan0;IPv4;Itunes\032Server\032on\032readynas;_daap._tcp;local;readynas.local;192.168.1.173;3689;"ffid=7e725542" "Password=false" "Version=196610" "iTSh Version=131073" "mtd-version=svn-1676" "Machine Name=Itunes Server" "Machine ID=6C37BA14" "Database ID=6C37BA14" "txtvers=1"
=;wlan0;IPv4;Itunes\032Server\032on\032readynas;_rsp._tcp;local;readynas.local;192.168.1.173;3689;"ffid=7e725542" "Password=false" "Version=196610" "iTSh Version=131073" "mtd-version=svn-1676" "Machine Name=Itunes Server" "Machine ID=6C37BA14" "Database ID=6C37BA14" "txtvers=1"
=;wlan0;IPv4;readynas\032\091a0\05821\058b7\058c1\0588c\0583a\093;_workstation._tcp;local;readynas.local;192.168.1.173;9;
=;wlan0;IPv4;root\064LibreELEC;_pulse-server._tcp;local;LibreELEC.local;192.168.1.227;4713;"cookie=0xcaa87808" "fqdn=LibreELEC" "uname=Linux armv7l 4.19.127 #1 SMP Tue Jul 6 19:08:28 CEST 2021" "machine-id=0591ddc66796df8c2bb9bd455b2cd975" "user-name=root" "server-version=pulseaudio 12.2"
=;wlan0;IPv4;Kodi\032\040LibreELEC\041;_xbmc-events._udp;local;LibreELEC.local;192.168.1.227;9777;
=;wlan0;IPv4;Kodi\032\040LibreELEC\041;_xbmc-jsonrpc._tcp;local;LibreELEC.local;192.168.1.227;9090;"uuid=58c530ac-0bb0-4f99-bbc6-c9d81639c72d" "txtvers=1"
=;wlan0;IPv4;Kodi\032\040LibreELEC\041;_xbmc-jsonrpc-h._tcp;local;LibreELEC.local;192.168.1.227;8080;"uuid=58c530ac-0bb0-4f99-bbc6-c9d81639c72d" "txtvers=1"
=;wlan0;IPv4;root\064LibreELEC\058\032Dummy\032Output;_pulse-sink._tcp;local;LibreELEC.local;192.168.1.227;4713;"icon-name=computer" "class=abstract" "description=Dummy Output" "subtype=virtual" "channel_map=front-left,front-right" "format=s16le" "channels=2" "rate=44100" "device=auto_null" "cookie=0xcaa87808" "fqdn=LibreELEC" "uname=Linux armv7l 4.19.127 #1 SMP Tue Jul 6 19:08:28 CEST 2021" "machine-id=0591ddc66796df8c2bb9bd455b2cd975" "user-name=root" "server-version=pulseaudio 12.2"
Failed to resolve service 'firefly [74:da:38:6e:5e:2a]' of type '_workstation._tcp' in domain 'local': Timeout reached
Failed to resolve service 'firefly [74:da:38:6e:5e:2a]' of type '_workstation._tcp' in domain 'local': Timeout reached
```


Office LAN
----------

```
$ avahi-browse -tarpk
+;eth2;IPv4;GIMLI;_device-info._tcp;local
+;br0;IPv6;GIMLI;_device-info._tcp;local
+;lxcbr0;IPv4;GIMLI;_device-info._tcp;local
+;virbr0;IPv6;GIMLI;_device-info._tcp;local
+;virbr0;IPv4;GIMLI;_device-info._tcp;local
+;wlan0;IPv6;GIMLI;_device-info._tcp;local
+;wlan0;IPv4;GIMLI;_device-info._tcp;local
+;eth0;IPv6;GIMLI;_device-info._tcp;local
+;eth0;IPv4;GIMLI;_device-info._tcp;local
+;lo;IPv4;GIMLI;_device-info._tcp;local
+;eth2;IPv4;GIMLI;_smb._tcp;local
+;br0;IPv6;GIMLI;_smb._tcp;local
+;lxcbr0;IPv4;GIMLI;_smb._tcp;local
+;virbr0;IPv6;GIMLI;_smb._tcp;local
+;virbr0;IPv4;GIMLI;_smb._tcp;local
+;wlan0;IPv6;GIMLI;_smb._tcp;local
+;wlan0;IPv4;GIMLI;_smb._tcp;local
+;eth0;IPv6;GIMLI;_smb._tcp;local
+;eth0;IPv4;GIMLI;_smb._tcp;local
+;lo;IPv4;GIMLI;_smb._tcp;local
+;eth0;IPv6;Brother\032DCP-L3550CDW\032series;_uscan._tcp;local
+;eth0;IPv4;Brother\032DCP-L3550CDW\032series;_uscan._tcp;local
+;eth0;IPv6;Brother\032DCP-L3550CDW\032series;_http._tcp;local
+;eth0;IPv4;Brother\032DCP-L3550CDW\032series;_http._tcp;local
+;eth0;IPv6;Brother\032DCP-L3550CDW\032series;_scanner._tcp;local
+;eth0;IPv4;Brother\032DCP-L3550CDW\032series;_scanner._tcp;local
+;eth0;IPv6;Brother\032DCP-L3550CDW\032series;_ipp._tcp;local
+;eth0;IPv4;Brother\032DCP-L3550CDW\032series;_ipp._tcp;local
+;eth0;IPv6;Brother\032DCP-L3550CDW\032series;_printer._tcp;local
+;eth0;IPv4;Brother\032DCP-L3550CDW\032series;_printer._tcp;local
+;eth0;IPv6;Brother\032DCP-L3550CDW\032series;_pdl-datastream._tcp;local
+;eth0;IPv4;Brother\032DCP-L3550CDW\032series;_pdl-datastream._tcp;local
=;br0;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::e018:99ff:fe36:51c7;0;"model=MacSamba"
=;br0;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::e018:99ff:fe36:51c7;445;
=;virbr0;IPv6;GIMLI;_device-info._tcp;local;gimli.local;2001:db8::1;0;"model=MacSamba"
=;virbr0;IPv6;GIMLI;_smb._tcp;local;gimli.local;2001:db8::1;445;
=;wlan0;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::fc94:34d5:b4d3:69e5;0;"model=MacSamba"
=;wlan0;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::fc94:34d5:b4d3:69e5;445;
=;eth0;IPv6;GIMLI;_device-info._tcp;local;gimli.local;fe80::fb59:6e2f:da3:975c;0;"model=MacSamba"
=;eth0;IPv6;GIMLI;_smb._tcp;local;gimli.local;fe80::fb59:6e2f:da3:975c;445;
=;eth2;IPv4;GIMLI;_device-info._tcp;local;gimli.local;192.168.2.1;0;"model=MacSamba"
=;eth2;IPv4;GIMLI;_smb._tcp;local;gimli.local;192.168.2.1;445;
=;lxcbr0;IPv4;GIMLI;_device-info._tcp;local;gimli.local;10.0.3.1;0;"model=MacSamba"
=;lxcbr0;IPv4;GIMLI;_smb._tcp;local;gimli.local;10.0.3.1;445;
=;virbr0;IPv4;GIMLI;_device-info._tcp;local;gimli.local;192.168.122.1;0;"model=MacSamba"
=;virbr0;IPv4;GIMLI;_smb._tcp;local;gimli.local;192.168.122.1;445;
=;wlan0;IPv4;GIMLI;_device-info._tcp;local;gimli.local;172.31.31.245;0;"model=MacSamba"
=;wlan0;IPv4;GIMLI;_smb._tcp;local;gimli.local;172.31.31.245;445;
=;eth0;IPv4;GIMLI;_device-info._tcp;local;gimli.local;172.31.21.164;0;"model=MacSamba"
=;eth0;IPv4;GIMLI;_smb._tcp;local;gimli.local;172.31.21.164;445;
=;lo;IPv4;GIMLI;_device-info._tcp;local;gimli.local;127.0.0.1;0;"model=MacSamba"
=;lo;IPv4;GIMLI;_smb._tcp;local;gimli.local;127.0.0.1;445;
=;eth0;IPv6;Brother\032DCP-L3550CDW\032series;_pdl-datastream._tcp;local;BRNB42200415BAA.local;172.31.21.11;9100;"UUID=e3248000-80ce-11db-8000-b42200415baa" "TBCP=T" "Transparent=F" "Binary=T" "PaperCustom=T" "Scan=T" "Fax=F" "Duplex=T" "Copies=T" "Color=T" "usb_CMD=PJL,PCL,PCLXL,URF" "usb_MDL=DCP-L3550CDW series" "usb_MFG=Brother" "priority=75" "adminurl=http://BRNB42200415BAA.local./" "product=(Brother DCP-L3550CDW series)" "ty=Brother DCP-L3550CDW series" "note=" "pdl=application/octet-stream,image/urf,image/jpeg,image/pwg-raster" "qtotal=1" "txtvers=1"
=;eth0;IPv6;Brother\032DCP-L3550CDW\032series;_printer._tcp;local;BRNB42200415BAA.local;172.31.21.11;515;"UUID=e3248000-80ce-11db-8000-b42200415baa" "TBCP=F" "Transparent=T" "Binary=T" "PaperCustom=T" "Scan=T" "Fax=F" "Duplex=T" "Copies=T" "Color=T" "usb_CMD=PJL,PCL,PCLXL,URF" "usb_MDL=DCP-L3550CDW series" "usb_MFG=Brother" "priority=50" "adminurl=http://BRNB42200415BAA.local./" "product=(Brother DCP-L3550CDW series)" "ty=Brother DCP-L3550CDW series" "note=" "rp=duerqxesz5090" "pdl=application/octet-stream,image/urf,image/jpeg,image/pwg-raster" "qtotal=1" "txtvers=1"
=;eth0;IPv6;Brother\032DCP-L3550CDW\032series;_ipp._tcp;local;BRNB42200415BAA.local;172.31.21.11;631;"mopria-certified=1.3" "print_wfds=T" "UUID=e3248000-80ce-11db-8000-b42200415baa" "PaperMax=legal-A4" "kind=document,envelope,label,postcard" "URF=SRGB24,W8,CP1,IS4-1,MT1-3-4-5-8-11,OB10,PQ4,RS600,V1.4,DM1" "TBCP=F" "Transparent=T" "Binary=T" "PaperCustom=T" "Scan=T" "Fax=F" "Duplex=T" "Copies=T" "Color=T" "usb_CMD=PJL,PCL,PCLXL,URF" "usb_MDL=DCP-L3550CDW series" "usb_MFG=Brother" "priority=25" "adminurl=http://BRNB42200415BAA.local./net/net/airprint.html" "product=(Brother DCP-L3550CDW series)" "ty=Brother DCP-L3550CDW series" "note=" "rp=ipp/print" "pdl=application/octet-stream,image/urf,image/jpeg,image/pwg-raster" "qtotal=1" "txtvers=1"
=;eth0;IPv6;Brother\032DCP-L3550CDW\032series;_scanner._tcp;local;BRNB42200415BAA.local;172.31.21.11;54921;"flatbed=T" "feeder=T" "button=T" "mdl=DCP-L3550CDW series" "mfg=Brother" "ty=Brother DCP-L3550CDW series" "adminurl=http://BRNB42200415BAA.local./" "note=" "txtvers=1"
=;eth0;IPv6;Brother\032DCP-L3550CDW\032series;_http._tcp;local;BRNB42200415BAA.local;172.31.21.11;80;
=;eth0;IPv6;Brother\032DCP-L3550CDW\032series;_uscan._tcp;local;BRNB42200415BAA.local;172.31.21.11;80;"duplex=F" "is=adf,platen" "cs=binary,grayscale,color" "UUID=e3248000-80ce-11db-8000-b42200415baa" "pdl=application/pdf,image/jpeg" "note=" "ty=Brother DCP-L3550CDW series" "rs=eSCL" "representation=http://BRNB42200415BAA.local./icons/device-icons-128.png" "adminurl=http://BRNB42200415BAA.local./net/net/airprint.html" "vers=2.63" "txtvers=1"
=;eth0;IPv4;Brother\032DCP-L3550CDW\032series;_pdl-datastream._tcp;local;BRNB42200415BAA.local;172.31.21.11;9100;"UUID=e3248000-80ce-11db-8000-b42200415baa" "TBCP=T" "Transparent=F" "Binary=T" "PaperCustom=T" "Scan=T" "Fax=F" "Duplex=T" "Copies=T" "Color=T" "usb_CMD=PJL,PCL,PCLXL,URF" "usb_MDL=DCP-L3550CDW series" "usb_MFG=Brother" "priority=75" "adminurl=http://BRNB42200415BAA.local./" "product=(Brother DCP-L3550CDW series)" "ty=Brother DCP-L3550CDW series" "note=" "pdl=application/octet-stream,image/urf,image/jpeg,image/pwg-raster" "qtotal=1" "txtvers=1"
=;eth0;IPv4;Brother\032DCP-L3550CDW\032series;_printer._tcp;local;BRNB42200415BAA.local;172.31.21.11;515;"UUID=e3248000-80ce-11db-8000-b42200415baa" "TBCP=F" "Transparent=T" "Binary=T" "PaperCustom=T" "Scan=T" "Fax=F" "Duplex=T" "Copies=T" "Color=T" "usb_CMD=PJL,PCL,PCLXL,URF" "usb_MDL=DCP-L3550CDW series" "usb_MFG=Brother" "priority=50" "adminurl=http://BRNB42200415BAA.local./" "product=(Brother DCP-L3550CDW series)" "ty=Brother DCP-L3550CDW series" "note=" "rp=duerqxesz5090" "pdl=application/octet-stream,image/urf,image/jpeg,image/pwg-raster" "qtotal=1" "txtvers=1"
=;eth0;IPv4;Brother\032DCP-L3550CDW\032series;_ipp._tcp;local;BRNB42200415BAA.local;172.31.21.11;631;"mopria-certified=1.3" "print_wfds=T" "UUID=e3248000-80ce-11db-8000-b42200415baa" "PaperMax=legal-A4" "kind=document,envelope,label,postcard" "URF=SRGB24,W8,CP1,IS4-1,MT1-3-4-5-8-11,OB10,PQ4,RS600,V1.4,DM1" "TBCP=F" "Transparent=T" "Binary=T" "PaperCustom=T" "Scan=T" "Fax=F" "Duplex=T" "Copies=T" "Color=T" "usb_CMD=PJL,PCL,PCLXL,URF" "usb_MDL=DCP-L3550CDW series" "usb_MFG=Brother" "priority=25" "adminurl=http://BRNB42200415BAA.local./net/net/airprint.html" "product=(Brother DCP-L3550CDW series)" "ty=Brother DCP-L3550CDW series" "note=" "rp=ipp/print" "pdl=application/octet-stream,image/urf,image/jpeg,image/pwg-raster" "qtotal=1" "txtvers=1"
=;eth0;IPv4;Brother\032DCP-L3550CDW\032series;_scanner._tcp;local;BRNB42200415BAA.local;172.31.21.11;54921;"flatbed=T" "feeder=T" "button=T" "mdl=DCP-L3550CDW series" "mfg=Brother" "ty=Brother DCP-L3550CDW series" "adminurl=http://BRNB42200415BAA.local./" "note=" "txtvers=1"
=;eth0;IPv4;Brother\032DCP-L3550CDW\032series;_http._tcp;local;BRNB42200415BAA.local;172.31.21.11;80;
=;eth0;IPv4;Brother\032DCP-L3550CDW\032series;_uscan._tcp;local;BRNB42200415BAA.local;172.31.21.11;80;"duplex=F" "is=adf,platen" "cs=binary,grayscale,color" "UUID=e3248000-80ce-11db-8000-b42200415baa" "pdl=application/pdf,image/jpeg" "note=" "ty=Brother DCP-L3550CDW series" "rs=eSCL" "representation=http://BRNB42200415BAA.local./icons/device-icons-128.png" "adminurl=http://BRNB42200415BAA.local./net/net/airprint.html" "vers=2.63" "txtvers=1"
```
