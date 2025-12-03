def main():
    import argparse
    import json
    import os
    import sys

    parser = argparse.ArgumentParser(prog='onieprom')

    parser.add_argument("infile", nargs="?", default=sys.stdin, type=argparse.FileType("rb", 0))
    parser.add_argument("outfile", nargs="?", default=sys.stdout, type=argparse.FileType("wb"))

    parser.add_argument("-e", "--encode", default=False, action="store_true",
                        help="Encode JSON input to binary output")

    parser.add_argument("-d", "--decode", default=False, action="store_true",
                        help="Decode binary input to JSON output")

    args = parser.parse_args()

    if (not args.encode) and (not args.decode):
        c = args.infile.read(1)
        args.infile.seek(0, 0)

        if c == b"{":
            args.encode = True
        elif c == b"T":
            args.decode = True
        else:
            sys.stderr.write("Neither encode nor decode specified, and could not infer operation from input")
            sys.exit(1)

    if args.encode:
        args.outfile.buffer.write(into_tlv(json.load(args.infile)))
    else:
        args.outfile.write(json.dumps(from_tlv(args.infile)))

if __name__ == "__main__":
    main()
