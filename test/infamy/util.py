import time

def until(fn, attempts=10, interval=1):
    for attempt in range(attempts):
        if fn():
            return

        time.sleep(interval)

    raise Exception("Expected condition did not materialize")
