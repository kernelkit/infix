DISTCLEANFILES      = *~ *.d
ACLOCAL_AMFLAGS     = -I m4

sbin_PROGRAMS       = netd
netd_SOURCES        = src/netd.c src/netd.h src/config.c src/config.h
netd_CPPFLAGS       = -D_DEFAULT_SOURCE -D_GNU_SOURCE -I$(srcdir)/src
netd_CFLAGS         = -W -Wall -Wextra
netd_CFLAGS        += $(libite_CFLAGS) $(libconfuse_CFLAGS) $(jansson_CFLAGS)
netd_LDADD          = $(libite_LIBS) $(libconfuse_LIBS) $(jansson_LIBS)

# Backend selection: FRR gRPC or Linux kernel
if HAVE_FRR_GRPC
BUILT_SOURCES = grpc/frr-northbound.pb.cc grpc/frr-northbound.pb.h \
                grpc/frr-northbound.grpc.pb.cc grpc/frr-northbound.grpc.pb.h

grpc/frr-northbound.pb.cc grpc/frr-northbound.pb.h: grpc/frr-northbound.proto
	$(AM_V_GEN)$(PROTOC) --cpp_out=grpc --proto_path=grpc $<

grpc/frr-northbound.grpc.pb.cc grpc/frr-northbound.grpc.pb.h: grpc/frr-northbound.proto
	$(AM_V_GEN)$(PROTOC) --grpc_out=grpc \
		--plugin=protoc-gen-grpc=$(GRPC_CPP_PLUGIN) \
		--proto_path=grpc $<

netd_SOURCES       += src/grpc_backend.cc src/grpc_backend.h \
                       src/json_builder.c src/json_builder.h \
                       grpc/frr-northbound.pb.cc grpc/frr-northbound.grpc.pb.cc
netd_CPPFLAGS      += $(grpc_CFLAGS) $(protobuf_CFLAGS)
netd_LDADD         += $(grpc_LIBS) $(protobuf_LIBS) -lstdc++

CLEANFILES = grpc/*.pb.cc grpc/*.pb.h
else
# Linux kernel backend (no FRR)
netd_SOURCES       += src/linux_backend.c src/linux_backend.h
endif
