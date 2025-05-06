class JsonData:
    @staticmethod
    def get(default, indata, *args):
        data = indata
        for arg in args:
            if arg in data:
                data = data.get(arg)
            else:
                return default

        return data


class Decore():
    @staticmethod
    def decorate(sgr, txt, restore="0"):
        return f"\033[{sgr}m{txt}\033[{restore}m"

    @staticmethod
    def invert(txt):
        return Decore.decorate("7", txt)

    @staticmethod
    def bold(txt):
        return Decore.decorate("1", txt)

    @staticmethod
    def red(txt):
        return Decore.decorate("31", txt, "39")

    @staticmethod
    def green(txt):
        return Decore.decorate("32", txt, "39")

    @staticmethod
    def yellow(txt):
        return Decore.decorate("33", txt, "39")

    @staticmethod
    def underline(txt):
        return Decore.decorate("4", txt, "24")

    @staticmethod
    def gray_bg(txt):
        return Decore.decorate("100", txt)
