"""
   Very basic mDNS scanner with HTML table renderer
"""
import subprocess

class MdnsHosts:
    """mDNS scanner class using avahi-browse"""
    def hask(self):
        """Check if avahi-browse has -k option"""
        try:
            result = subprocess.run(['avahi-browse', '--help'], check=True,
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            return '-k' in result.stdout
        except subprocess.CalledProcessError:
            return False

    def scan(self):
        """Perform mDNS scan and return list of hosts."""
        services = {
            '_daap._tcp':        ('iTunes Server', None),
            '_device-info._tcp': ('Device info', None),
            '_http._tcp':        ('HTTP', 'http://{address}'),
            '_https._tcp':       ('HTTPS', 'https://{address}'),
            '_ipp._tcp':         ('Printer', None),
            '_ipps._tcp':        ('Printer', None),
            '_netconf-ssh._tcp': ('NETCONF', None),
            '_raop._tcp':        ('AirPlay ', None),
            '_ssh._tcp':         ('SSH', None),
            '_sftp-ssh._tcp':    ('SFTP', None),
            '_workstation._tcp': ('Workstation', None),
        }

        result = subprocess.run(['avahi-browse', '-tarpk' if self.hask() else '-tarp'],
                                stdout=subprocess.PIPE, text=True)
        lines = result.stdout.strip().split('\n')
        hosts_services = {}  # Key: address, Value: list of dicts with service details

        for line in lines:
            print(f"{line}")
            parts = line.split(';')
            if len(parts) <= 8 or parts[0] != '=':
                continue
            if (parts[2] != 'IPv4' and parts[2] != 'IPv6'):
                continue

            service_type = parts[4]
            if service_type not in services:
                continue

            service_name = parts[3]
            address = parts[7]
            identifier, url_template = services[service_type]
            url = url_template.format(address=address) if url_template else None

            service_details = {
                'name': self.decode(service_name),
                'type': identifier,
                'link': parts[6],
                'url': url
            }
            if address not in hosts_services:
                hosts_services[address] = {'services': [service_details], 'type': parts[2]}
            else:
                hosts_services[address]['services'].append(service_details)

        return hosts_services

    def decode(self, name):
        """Decode escape sequences like \032 and \040 in service names"""
        name = name.replace('\\032', ' ')
        name = name.replace('\\040', '(')
        name = name.replace('\\041', ')')
        return bytes(name, "utf-8").decode("unicode_escape")

    def html(self):
        """Generate a HTML table of the mDNS scan results"""
        hosts_services = self.scan()
        html_content = """
    <html>
    <head>
    <title>mDNS Hosts - Services</title>
    <style>
        .container {
            max-width: 1024px;
            margin: auto;
            padding: 0 10px;
        }
        body { font-family: Arial, sans-serif; margin: 20px; }
        table { border-collapse: collapse; width: 100%; }
        th, td { text-align: left; padding: 8px; border-bottom: 1px solid #ddd; }
        th { background-color: #f2f2f2; }
        tr:hover {background-color: #e8e8e8;}
    </style>
    </head>
    <body>
    <div class="container">
    <input type="text" id="search" onkeyup="filter()" placeholder="Filter column ...">
    <table id="main">
    <tr><th>IP Address</th><th>Service Name</th><th>Link</th><th>Service Type</th></tr>
    """

        for address, info in hosts_services.items():
            for service in info['services']:
                service_name = service['name']
                service_type = service['type']
                link = service['link']
                url = service['url']
                if url:
                    url       = url.replace("127.0.0.1", link)
                    link_html = f'<a href="{url}">{link}</a>'
                else:
                    link_html = f'{link}'

                html_content += f"<tr><td>{address}</td><td>{service_name}</td>"
                html_content += f"<td>{link_html}</td><td>{service_type}</td></tr>\n"

        html_content += """
    </table>
    </div>
    <script>
        function filter() {
            var input, filter, table, tr, tdaddr, tdserv, i, addr, serv;
            input = document.getElementById("search");
            filter = input.value.toUpperCase();
            table = document.getElementById("main");
            tr = table.getElementsByTagName("tr");

            for (i = 1; i < tr.length; i++) {
                tdaddr = tr[i].getElementsByTagName("td")[0];
                tdserv = tr[i].getElementsByTagName("td")[1];
                tdlink = tr[i].getElementsByTagName("td")[2];
                tdtype = tr[i].getElementsByTagName("td")[3];
                addr = tdaddr ? tdaddr.textContent || tdaddr.innerText : "";
                serv = tdserv ? tdserv.textContent || tdserv.innerText : "";
                link = tdlink ? tdlink.textContent || tdlink.innerText : "";
                type = tdtype ? tdtype.textContent || tdtype.innerText : "";

                if (addr.toUpperCase().indexOf(filter) > -1 ||
                    serv.toUpperCase().indexOf(filter) > -1 ||
                    link.toUpperCase().indexOf(filter) > -1 ||
                    type.toUpperCase().indexOf(filter) > -1) {
                    tr[i].style.display = "";
                } else {
                    tr[i].style.display = "none";
                }
            }
        }
    </script>
    </body>
    </html>
    """
        return html_content
