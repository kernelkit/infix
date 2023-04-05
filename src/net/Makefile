LDLIBS = -lite

net: main.o
	$(CC) -o $@ $^ $(LDLIBS)

clean:
	$(RM) net

distclean: clean
	$(RM) *.o *~
