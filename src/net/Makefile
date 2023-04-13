LDLIBS = -lite
EXEC   = net

net: main.o
	$(CC) -o $@ $^ $(LDLIBS)

check: $(EXEC)
	./test.sh

clean:
	$(RM) net

distclean: clean
	$(RM) *.o *~
