include Makefile

install: $(addprefix $(DESTDIR)/boot/,$(dtb-y))

$(DESTDIR)/boot/%.dtb: %.dtb
	@echo "  DTB-INSTALL $<"
	@install -D $< $@

$(DESTDIR)/boot/%.dtbo: %.dtbo
	@echo "  DTBO-INSTALL $<"
	@install -D $< $@

.PHONY: install
