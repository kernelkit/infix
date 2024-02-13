Vital Product Data
==================

Infix can source Vital Product Data (VPD) from EEPROMs containing an
[ONIE TLV][oniespec]. The load bearing information from VPDs, used by
Infix by default, is:

1. Base (chassis) MAC address of the system
2. Factory default admin password hash

Additionally, every VPD is listed as a separate _component_ in the
_ietf-hardware_ model, in which Infix exports all available attributes
to the user. This is useful to read out exact hardware revisions of
each board making up a system, figure out if a system belongs to a
particular production batch, etc.


## JSON Encoding

To make EEPROM binary generation less cumbersome, Infix defines a JSON
encoding of an ONIE TLV, and provides a tool, [onieprom], to translate
from one encoding to another. I.e. a JSON object can be translated to
its binary equivalent, and vice versa.

A mapping between TLV attribute IDs and JSON keys is defined, using
the Linux kernel [driver] as a reference:

| TLV ID | Description      | Key                  | Type                            |
|--------|------------------|----------------------|---------------------------------|
| `0x21` | Product Name     | `"product-name"`     | String                          |
| `0x22` | Part Number      | `"part-number"`      | String                          |
| `0x23` | Serial Number    | `"serial-number"`    | String                          |
| `0x24` | MAC #1 Base      | `"mac-address"`      | String ("c0:ff:ee:00:00:00")    |
| `0x25` | Manufacture Date | `"manufacture-date"` | String ("MM/DD/YYYY HH:NN:SS")  |
| `0x26` | Device Version   | `"device-version"`   | Integer (u8)                    |
| `0x27` | Label Revision   | `"label-revision"`   | String                          |
| `0x28` | Platform Name    | `"platform-name"`    | String                          |
| `0x29` | ONIE Version     | `"onie-version"`     | String                          |
| `0x2a` | Num MACs         | `"num-macs"`         | Integer (u16)                   |
| `0x2b` | Manufacturer     | `"manufacturer"`     | String                          |
| `0x2c` | Country Code     | `"country-code"`     | String (ISO 3166-1 2-byte code) |
| `0x2d` | Vendor           | `"vendor"`           | String                          |
| `0x2e` | Diag Version     | `"diag-version"`     | String                          |
| `0x2f` | Service Tag      | `"service-tag"`      | String                          |
| `0xfd` | Vendor Extension | `"vendor-extension"` | List of extensions (see below)  |

The JSON encoding of a TLV is a single JSON object with an
arbitrary subset of the attributes from the list above.

**Example:**
```json
{
	"product-name": "Wacky Widget",
	"serial-number": "#1",
	"manufacture-date": "02/13/2024 11:29:52"
}
```

### Vendor Extensions

As the specification explicitly mentions the option of supplying
multiple vendor extensions, possibly multiple ones of the same type,
the JSON chosen encoding is a list, where each element is itself a
list of exactly two elements:

| Element # | Type                                          |
|-----------|-----------------------------------------------|
| 1         | Integer (u32, [IANA enterprise number][pens]) |
| 2         | String (UTF-8 encoded extension data)         |

The format of the extension data is defined by the entity indicated by
the IANA enterprise number. Restricting extension data to UTF-8
encoded strings is a decision imposed by Infix; the ONIE specification
allows for arbitrary binary data.

**Example**:
```json
{
	...
	"vendor-extension": [
		[ 12345, "my extension data" ]
	]
}
```

### Infix Specific Extensions

[Kernelkit][kkit]'s IANA enterprise number is `61046`, under which any
extensions required by Infix are stored. The **only** valid extension
data that may be stored under this number is documented in _this
section_. If other device specific data needs to be stored in a VPD,
you must associate that with an enterprise number under our control.

Every Kernelkit extension must be a valid JSON object containing an
arbitrary subset of the following attributes:

| Key        | Value                            | Description                          |
|------------|----------------------------------|--------------------------------------|
| `"pwhash"` | String (output of `mkpasswd(1)`) | Factory default password for `admin` |

Since the extension is itself stored in a JSON document, it has to be
appropriately quoted.

**Example**:
```json
{
	...
	"vendor-extension": [
		[ 61046, "{\"pwhash\":\"$6$9rufAxdqCrxrwfQR$G0l9cTVlu/vOhxgo/uMKfRDOmZRd5XWF3vKr5da6qYoxuTJBS/Pl9K.5lrabWoWFFc.71yFMaSlZz0O8FtAtl.\"}" ]
	]
}
```

## Creating and Parsing ONIE EEPROM Binaries

```
usage: onieprom [-h] [-e] [-d] [infile] [outfile]

positional arguments:
  infile
  outfile

options:
  -h, --help    show this help message and exit
  -e, --encode  Encode JSON input to binary output
  -d, --decode  Decode binary input to JSON output
```

To convert a compatible JSON document (using the first example above)
to its binary equivalent, we ask [onieprom] to _encode_ it for us:

```sh
~$ onieprom -e example.json >example.bin
~$ hexdump -C example.bin
00000000  54 6c 76 49 6e 66 6f 00  01 00 2d 25 13 30 32 2f  |TlvInfo...-%.02/|
00000010  31 33 2f 32 30 32 34 20  31 31 3a 32 39 3a 35 32  |13/2024 11:29:52|
00000020  21 0c 57 61 63 6b 79 20  57 69 64 67 65 74 23 02  |!.Wacky Widget#.|
00000030  23 31 fe 04 dd 69 88 97                           |#1...i..|
00000038
```

We can also run the process in reverse, to inspect the contents of a
binary in its equivalent JSON representation:

```sh
~$ onieprom -d example.bin >example-again.json
~$ jq . example-again.json
{
  "manufacture-date": "02/13/2024 11:29:52",
  "product-name": "Wacky Widget",
  "serial-number": "#1"
}
```


[oniespec]: https://opencomputeproject.github.io/onie/design-spec/hw_requirements.html
[onieprom]: ../board/common/rootfs/bin/onieprom
[driver]: https://elixir.bootlin.com/linux/latest/source/drivers/nvmem/layouts/onie-tlv.c
[pens]: https://www.iana.org/assignments/enterprise-numbers/
[kkit]: https://github.com/kernelkit
