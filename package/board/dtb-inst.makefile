include Makefile

install: $(addprefix $(DESTDIR)/boot/,$(dtb-y))

$(DESTDIR)/boot/%.dtb: %.dtb
	@echo "  DTB-INSTALL $<"
	@install -D $< $@

.PHONY: install
