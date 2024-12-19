import contextlib
import datetime
import subprocess
import sys
import traceback

import infamy.netns

class Test:
    def __init__(self, output=sys.stdout):
        self.out = output
        self.commenter = CommentWriter(self.out)
        if self.out == sys.stdout:
            sys.stdout = self.commenter

        self.test_cleanup=[]
        self.steps = 0

    def push_test_cleanup(self, fn):
        self.test_cleanup.append(fn)

    def cleanup(self):
        infamy.netns.IsolatedMacVlans.Cleanup()
        for test_cleanup in reversed(self.test_cleanup):
            test_cleanup()

    def __enter__(self):
        now = datetime.datetime.now().strftime("%F %T")
        self.out.write(f"# Starting ({now})\n")
        self.out.flush()
        return self

    def __exit__(self, _, e, __):
        now = datetime.datetime.now().strftime("%F %T")
        self.out.write(f"# Exiting ({now})\n")
        self.out.flush()

        self.cleanup()

        if not e:
            self._not_ok("Missing explicit test result\n")
        else:
            if type(e) in (TestPass, TestSkip):
                self.out.write(f"{self.steps}..{self.steps}\n")
                self.out.flush()
                raise SystemExit(0)

            traceback.print_exception(e, file=self.commenter)

            if type(e) is subprocess.CalledProcessError:
                print("Failing subprocess stdout:\n", e.stdout)
            elif len(e.args) and type(e.args[0]) is subprocess.CompletedProcess:
                print("Failing subprocess stdout:\n", e.args[0].stdout)

        raise SystemExit(1)

    @contextlib.contextmanager
    def step(self, msg):
        try:
            yield
            self._ok(msg)

        except Exception as e:
            if type(e) == TestPass:
                self._ok(msg)
            elif type(e) == TestSkip:
                self._ok(directive="skip")
            elif type(e) == TestFail:
                self._not_ok(msg)
            else:
                self._not_ok(msg)

            raise e

    def _report(self, tag, msg):
        self.steps += 1
        self.out.write(f"{tag} {self.steps}{msg}\n")
        self.out.flush()

    def _ok(self, msg="", directive=None):
        if msg:
            msg = " - " + msg

        if directive:
            msg = msg + " # " + directive

        self._report("ok", msg)

    def _not_ok(self, msg):
        self._report("not ok", " - " + msg)

    def succeed(self):
        raise TestPass()

    def skip(self):
        raise TestSkip()

    def fail(self):
        raise TestFail()


class TestResult(Exception):
    pass


class TestPass(TestResult):
    pass


class TestFail(TestResult):
    pass


class TestSkip(TestResult):
    pass


class CommentWriter:
    def __init__(self, f):
        self.f = f
        self.at_nl = True

    def write(self, data):
        if self.at_nl:
            data = "# " + data
            self.at_nl = False
            if not len(data):
                return

        if data.endswith("\n"):
            self.at_nl = True
            data = data[:-1]

        data = data.replace("\n", "\n# ")
        if self.at_nl:
            data = data + "\n"

        self.f.write(data)
        self.flush()

    def flush(self):
        return self.f.flush()
