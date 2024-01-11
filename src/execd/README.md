Execute jobs on route changes
=============================

This is a generic job queue executor for work that needs network access.

For example creating a Docker container by downloading an image from the
network -- if the download fails `execd` retries the job whenever there
is a route change.

