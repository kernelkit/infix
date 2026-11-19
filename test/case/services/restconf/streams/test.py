#!/usr/bin/env python3
"""
RESTCONF event stream

Verify that a client can subscribe to notifications over RESTCONF
without sending a Forward header, that the event stream stays open
while nothing happens, and that it then delivers an event.
"""
import time

import infamy
import infamy.sse as sse


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt", "restconf")
        host = target.get_mgmt_ip()

    with test.step("Subscribe to the NETCONF stream without a Forward header"):
        uri = sse.subscribe(target, {
            "stream": "NETCONF",
            "encoding": "encode-json"
        })

    with test.step("Verify the reply has a stream URI on the address of the DUT"):
        address = host.split("%")[0]
        assert uri.startswith(f"https://[{address}]/streams/subscribed/"), uri

    with test.step("Open the event stream from the host"):
        stream = sse.Stream(target, uri)

    with test.step("Verify the stream is still open after 150 seconds without events"):
        time.sleep(150)
        assert not stream.closed.is_set()
        assert stream.events.empty()

    with test.step("Set hostname to stream-test"):
        target.put_config_dicts({
            "ietf-system": {
                "system": {
                    "hostname": "stream-test"
                }
            }
        })

    with test.step("Verify the stream delivers a netconf-config-change event"):
        assert stream.wait_for(lambda event: "ietf-netconf-notifications:netconf-config-change" in event, 10)

    test.succeed()
