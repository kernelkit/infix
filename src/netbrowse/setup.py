from setuptools import setup, find_packages

setup(
    packages=find_packages(),
    scripts=['netbrowse'],
    install_requires=[
        'dbus-python',
        'flup'
    ],
)
