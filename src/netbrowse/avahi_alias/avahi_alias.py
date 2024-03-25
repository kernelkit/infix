"""Avahi CNAME alias class"""

from encodings.idna import ToASCII
import dbus

class AvahiAlias:
    """Import to your project and use publish_cname() for each name."""
    DBUS_NAME = 'org.freedesktop.Avahi'
    CLASS_IN = 0x01
    TYPE_CNAME = 0x05
    TTL = 60
    DBUS_PATH_SERVER = '/'
    DBUS_INTERFACE_SERVER = DBUS_NAME + '.Server'
    DBUS_INTERFACE_ENTRY_GROUP = DBUS_NAME + '.EntryGroup'
    IF_UNSPEC = -1
    PROTO_UNSPEC = -1

    def publish_cname(self, cname):
        """Call this method for each alias to publish as a CNAME record."""
        bus = dbus.SystemBus()
        server = dbus.Interface(bus.get_object(self.DBUS_NAME, self.DBUS_PATH_SERVER),
                                self.DBUS_INTERFACE_SERVER)
        group = dbus.Interface(bus.get_object(self.DBUS_NAME, server.EntryGroupNew()),
                               self.DBUS_INTERFACE_ENTRY_GROUP)

        rdata = self.create_rr(server.GetHostNameFqdn())
        cname = self.encode_dns(cname)

        group.AddRecord(self.IF_UNSPEC, self.PROTO_UNSPEC, dbus.UInt32(0),
                        cname, self.CLASS_IN, self.TYPE_CNAME, self.TTL, rdata)
        group.Commit()
        print("Published " + cname.decode())

    @staticmethod
    def encode_dns(name):
        """Internal"""
        out = []
        name = name.decode()
        for part in str(name).split('.'):
            if len(part) == 0:
                continue
            out.append(ToASCII(part))
        return b'.'.join(out)

    @staticmethod
    def create_rr(name):
        """Internal"""
        out = []
        for part in name.split('.'):
            if len(part) == 0:
                continue
            out.append(chr(len(part)).encode())
            out.append(ToASCII(part))
        out.append(b'\0')

        return b''.join(out)
