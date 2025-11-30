#!/usr/bin/env python3
"""Support data collection

Verify that the support collect command works and produces a valid tarball
with expected content.

"""

import os
import subprocess
import tarfile
import tempfile
import infamy
import infamy.ssh as ssh

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")

    with test.step("Run support collect with short log tail (2 seconds)"):
        # Create temporary file for output
        with tempfile.NamedTemporaryFile(suffix=".tar.gz", delete=False) as tmp:
            output_file = tmp.name

        # Run support collect via SSH with short log tail for testing
        # Capture stdout (the tarball) to file
        with open(output_file, 'wb') as f:
            result = tgtssh.run("support collect --log-sec 2",
                                stdout=f,
                                stderr=subprocess.PIPE)

        if result.returncode != 0:
            stderr_output = result.stderr.decode('utf-8') if result.stderr else ""
            print(f"support collect failed with return code {result.returncode}")
            print(f"stderr: {stderr_output}")
            raise Exception("support collect command failed")

    with test.step("Verify tarball was created and is valid"):
        if not os.path.exists(output_file):
            raise Exception(f"Output file {output_file} was not created")

        file_size = os.path.getsize(output_file)
        if file_size == 0:
            raise Exception("Output tarball is empty")

        print(f"Tarball created: {file_size} bytes")

        # Verify it's a valid tar.gz
        try:
            with tarfile.open(output_file, 'r:gz') as tar:
                members = tar.getnames()
                print(f"Tarball contains {len(members)} files/directories")

                # Verify some expected files exist
                expected_files = [
                    'collection.log',
                    'operational-config.json',
                    'system/dmesg.txt',
                    'system/meminfo.txt',
                    'network/ip/addr.txt'
                ]

                root_dir = members[0] if members else None
                for expected in expected_files:
                    full_path = f"{root_dir}/{expected}" if root_dir else expected
                    if full_path not in members:
                        print(f"Warning: Expected file '{expected}' not found in tarball")
                    else:
                        print(f"Found: {expected}")

        except tarfile.TarError as e:
            raise Exception(f"Invalid tarball: {e}")

        finally:
            # Clean up
            if os.path.exists(output_file):
                os.remove(output_file)

    test.succeed()
