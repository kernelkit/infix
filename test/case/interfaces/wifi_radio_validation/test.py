#!/usr/bin/env python3
r"""
WiFi radio settings the hardware cannot do are refused

A radio reports the channels it supports.  A channel outside the
configured band and a channel the country code does not allow must be
refused when configured, with an error that names the radio and the
setting.  A setting the radio does support must still be accepted
afterwards.

Topology:
....
    host ==(mgmt)== target  )))  ~ cell ~
....
"""
import infamy
import infamy.wifi as wifi
from infamy.util import until


def must_reject(target, radio, reason):
    """The commit must fail, and the error must name the reason"""
    try:
        target.put_config_dicts({"ietf-hardware": wifi.hardware(radio)})
    except Exception as err:
        text = getattr(getattr(err, "response", None), "text", None) or str(err)
        assert reason in text, f"rejected for another reason:\n{text}"
        print(f"Rejected as expected: {reason}")
        return
    raise AssertionError("configuration was accepted")


def supported_channels(target):
    data = target.get_data("/ietf-hardware:hardware/component[name='radio0']")
    radio = data["hardware"]["component"]["radio0"]
    radio = radio.get("infix-hardware:wifi-radio") or radio.get("wifi-radio") or {}
    return radio.get("supported-channels", [])


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        wifi.skip_unless_supported(test, target)

    with test.step("Configure radio0 for 2.4GHz channel 1 with country code SE"):
        target.put_config_dicts({
            "ietf-hardware": wifi.hardware(wifi.radio("radio0", band="2.4GHz", channel=1)),
        })

    with test.step("Verify radio0 supports channel 13 but not channel 14"):
        until(lambda: 13 in supported_channels(target) and
              14 not in supported_channels(target), attempts=30)

    with test.step("Reject channel 36 in band 2.4GHz on radio0"):
        must_reject(target, wifi.radio("radio0", band="2.4GHz", channel=36),
                    "radio0: channel 36 is not a 2.4GHz channel")

    with test.step("Reject channel 14 in band 2.4GHz on radio0"):
        must_reject(target, wifi.radio("radio0", band="2.4GHz", channel=14),
                    "radio0: channel 14 is not available on this radio")

    with test.step("Configure radio0 for 2.4GHz channel 13"):
        target.put_config_dicts({
            "ietf-hardware": wifi.hardware(wifi.radio("radio0", band="2.4GHz", channel=13)),
        })

    test.succeed()
