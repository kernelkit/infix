Checklist for Updating Docker Image
===================================

This directory holds the Dockerfile and any extras needed to build and
update the infix-test container image used for the test system.

The following is a checklist/reminder to maintainers for how to update
the image, e.g., with missing Alpine packages.

 1. Update the Dockerfile

        cd test/docker/
        edit Dockerfile

 2. Build the new image version, for latest version, see released images
    here: <https://github.com/kernelkit/infix/pkgs/container/infix-test>
	in this example we use version 0.4:

        docker build -t ghcr.io/kernelkit/infix-test:0.4 .

 3. Update the `test/env` file to use the new version
 4. Verify your new image works properly (remember to remove your `~/.infix-test-venv`)
 5. Send PR to co-maintainer for review

The co-maintainer should then verify themselves before approving the PR.
A crucial step to remember is to:

 1. Push the new image version to <https://ghcr.io>.  For details on how
    to do this, see this excellent guide to the [GitHub Container
    Registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry):
 2. Shorthand after setup (above):

         echo $CR_PAT | docker login ghcr.io -u troglobit --password-stdin
         docker push ghcr.io/kernelkit/infix-test:0.4

 3. Merge the PR to the `main` branch.

> **Note:** the co-maintainer may delegate the chore of uploading the
> new image to the one who prepared the PR (you), provided of course
> they have the access rights to do so.
