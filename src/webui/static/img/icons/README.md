# Sidebar icons

All icons in this directory are from [Lucide][1] (ISC-licensed),
vendored locally so the WebUI works on air-gapped industrial sites
without an external CDN.

Each file is the upstream `<lucide>/icons/<name>.svg` with the `width`
and `height` attributes stripped so the icon scales to its CSS-defined
size in the sidebar.  Stroke styling (`fill="none"`,
`stroke="currentColor"`, `stroke-width="2"`, rounded caps/joins) is left
untouched.

| File              | Lucide source         |
|-------------------|-----------------------|
| advanced.svg      | folder-git-2          |
| backup.svg        | archive-restore       |
| console.svg       | square-terminal       |
| containers.svg    | container             |
| dashboard.svg     | circle-gauge          |
| dhcp.svg          | network               |
| diagnostics.svg   | activity              |
| dns.svg           | globe                 |
| download.svg      | download              |
| firewall.svg      | shield-check          |
| hardware.svg      | cpu                   |
| interfaces.svg    | ethernet-port         |
| keystore.svg      | key-round             |
| lldp.svg          | radio-tower           |
| logs.svg          | scroll-text           |
| mdns.svg          | radio                 |
| nacm.svg          | users                 |
| ntp.svg           | clock                 |
| routes.svg        | git-compare-arrows    |
| routing.svg       | git-compare-arrows    |
| services.svg      | library-big           |
| software.svg      | hard-drive-download   |
| status-tree.svg   | text-search           |
| system-control.svg| power                 |
| system.svg        | settings              |
| wifi.svg          | wifi                  |
| wireguard.svg     | globe-lock            |

## Replacing or adding an icon

1. Pick the icon at [lucide.dev/icons][2].
2. Download the SVG (or copy from `lucide-icons/lucide`'s `icons/`
   directory on GitHub).
3. Drop the `width` and `height` attributes from the opening `<svg>`
   tag.
4. Save under the destination filename (the sidebar template references
   files by purpose, not by upstream Lucide name).
5. Update the table above.

## License

Lucide is distributed under the [ISC license][3].  The license text is
preserved in the upstream repository.

[1]: https://lucide.dev
[2]: https://lucide.dev/icons/
[3]: https://github.com/lucide-icons/lucide/blob/main/LICENSE
