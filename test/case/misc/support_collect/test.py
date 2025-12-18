#!/usr/bin/env python3
"""Support data collection

Verify that the support collect command works and produces a valid tarball
with expected content. Tests both the --work-dir global option and GPG
encryption (when available on target).

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

    with test.step("Check for GPG availability on target"):
        result = tgtssh.run("command -v gpg >/dev/null 2>&1", check=False)
        has_gpg = (result.returncode == 0)
        if has_gpg:
            print("GPG is available on target - will test encryption")
        else:
            print("GPG not available on target - skipping encryption tests")

    with test.step("Run support collect with --work-dir and short log tail"):
        # Create temporary file for output
        with tempfile.NamedTemporaryFile(suffix=".tar.gz", delete=False) as tmp:
            output_file = tmp.name

        # Use /tmp as work-dir to test the --work-dir option
        # Run support collect via SSH with short log tail for testing
        # Capture stdout (the tarball) to file
        # Note: timeout is generous to handle systems with many network ports
        # (ethtool collection scales with number of interfaces)
        with open(output_file, 'wb') as f:
            result = tgtssh.run("sudo support --work-dir /tmp collect --log-sec 2",
                                stdout=f,
                                stderr=subprocess.PIPE,
                                timeout=120)

        if result.returncode != 0:
            stderr_output = result.stderr.decode('utf-8') if result.stderr else ""
            print(f"support collect failed with return code {result.returncode}")
            print(f"stderr: {stderr_output}")

            # Try to retrieve the collection.log for debugging
            print("\n=== Attempting to retrieve collection.log for debugging ===")
            try:
                log_result = tgtssh.run("find /tmp -name 'support-*' -type d -exec cat {}/collection.log \\; 2>/dev/null || echo 'No collection.log found'",
                                       stdout=subprocess.PIPE,
                                       stderr=subprocess.PIPE,
                                       timeout=10,
                                       check=False)
                if log_result.stdout:
                    log_output = log_result.stdout.decode('utf-8')
                    print(f"collection.log contents:\n{log_output}")
            except Exception as e:
                print(f"Could not retrieve collection.log: {e}")

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
                    'network/ip/addr.json'
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

    if has_gpg:
        with test.step("Run support collect with GPG encryption"):
            # Create temporary file for encrypted output
            with tempfile.NamedTemporaryFile(suffix=".tar.gz.gpg", delete=False) as tmp:
                encrypted_file = tmp.name

            # Use a test password
            test_password = "test-support-password-123"

            # Run support collect with encryption
            with open(encrypted_file, 'wb') as f:
                result = tgtssh.run(f"sudo support --work-dir /tmp collect --log-sec 2 --password {test_password}",
                                    stdout=f,
                                    stderr=subprocess.PIPE,
                                    timeout=120)

            if result.returncode != 0:
                stderr_output = result.stderr.decode('utf-8') if result.stderr else ""
                print(f"support collect with encryption failed: {stderr_output}")

                # Try to retrieve the collection.log for debugging
                print("\n=== Attempting to retrieve collection.log for debugging ===")
                try:
                    log_result = tgtssh.run("find /tmp -name 'support-*' -type d -exec cat {}/collection.log \\; 2>/dev/null || echo 'No collection.log found'",
                                           stdout=subprocess.PIPE,
                                           stderr=subprocess.PIPE,
                                           timeout=10,
                                           check=False)
                    if log_result.stdout:
                        log_output = log_result.stdout.decode('utf-8')
                        print(f"collection.log contents:\n{log_output}")
                except Exception as e:
                    print(f"Could not retrieve collection.log: {e}")

                raise Exception("support collect with --password failed")

        with test.step("Verify encrypted file and decrypt it"):
            if not os.path.exists(encrypted_file):
                raise Exception(f"Encrypted output file {encrypted_file} was not created")

            file_size = os.path.getsize(encrypted_file)
            if file_size == 0:
                raise Exception("Encrypted output file is empty")

            print(f"Encrypted file created: {file_size} bytes")

            # Create temporary file for decrypted output
            with tempfile.NamedTemporaryFile(suffix=".tar.gz", delete=False) as tmp:
                decrypted_file = tmp.name

            try:
                # Decrypt the file using gpg
                with open(encrypted_file, 'rb') as ef:
                    with open(decrypted_file, 'wb') as df:
                        decrypt_result = subprocess.run(
                            ["gpg", "--batch", "--yes", "--passphrase", test_password,
                             "--pinentry-mode", "loopback", "-d"],
                            stdin=ef,
                            stdout=df,
                            stderr=subprocess.PIPE,
                            timeout=30
                        )

                if decrypt_result.returncode != 0:
                    stderr_output = decrypt_result.stderr.decode('utf-8') if decrypt_result.stderr else ""
                    print(f"GPG decryption failed: {stderr_output}")
                    raise Exception("Failed to decrypt GPG-encrypted support data")

                print("Successfully decrypted GPG file")

                # Verify the decrypted file is a valid tarball
                with tarfile.open(decrypted_file, 'r:gz') as tar:
                    members = tar.getnames()
                    print(f"Decrypted tarball contains {len(members)} files/directories")

                    # Verify some expected files exist
                    expected_files = [
                        'collection.log',
                        'operational-config.json',
                        'system/dmesg.txt'
                    ]

                    root_dir = members[0] if members else None
                    for expected in expected_files:
                        full_path = f"{root_dir}/{expected}" if root_dir else expected
                        if full_path not in members:
                            print(f"Warning: Expected file '{expected}' not found in decrypted tarball")
                        else:
                            print(f"Found in decrypted tarball: {expected}")

            except tarfile.TarError as e:
                raise Exception(f"Decrypted file is not a valid tarball: {e}")

            except subprocess.TimeoutExpired:
                raise Exception("GPG decryption timed out")

            except FileNotFoundError:
                print("Warning: gpg not available on host system - skipping decryption verification")

            finally:
                # Clean up
                if os.path.exists(encrypted_file):
                    os.remove(encrypted_file)
                if os.path.exists(decrypted_file):
                    os.remove(decrypted_file)

    test.succeed()
