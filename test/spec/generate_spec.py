#!/bin/env python3
import os
import ast
import graphviz
import argparse

from pathlib import Path

# Define a custom NodeVisitor to extract the arguments
class TestStepVisitor(ast.NodeVisitor):
    def __init__(self):
        self.test_steps=[]
        self.name = ""
        self.description = ""

    def visit_Module(self, node):
        """Extract docstring from a module."""
        docstring = ast.get_docstring(node)
        if docstring:
            lines = docstring.splitlines()

            newline_parsed=False

            for line in lines:
                if self.name == "":
                    self.name=line.strip()
                elif newline_parsed == False and line == "":
                    newline_parsed = True
                    continue # Skip mandatory newline
                else:
                    self.description=f"{self.description}\n{line}"
            self.description=self.description.strip()
        self.generic_visit(node)

    # Check for test.step() for the actual test steps
    def visit_Call(self, node):
        if isinstance(node.func, ast.Attribute) and node.func.attr == 'step':
            if isinstance(node.func.value, ast.Name) and node.func.value.id == 'test':
                if node.args and isinstance(node.args[0], ast.Constant):
                    self.test_steps.append(node.args[0].value)
        self.generic_visit(node)  # Continue visiting other nodes

class TestCase:
    def __init__(self, directory, rootdir=None):
        self.test_dir=Path(directory)
        if rootdir:
            rootdir=Path(f"{rootdir}")
            self.test_dir=self.test_dir.relative_to(rootdir)
        self.topology_dot=f"{directory}/topology.dot"
        self.topology_image=f"{directory}/topology"
        self.test_case=f"{directory}/test.py"
        self.specification=f"{directory}/Readme.adoc"
        with open(self.test_case, 'r') as file:
            script_content = file.read()
        parsed_script = ast.parse(script_content)
        visitor = TestStepVisitor()
        visitor.visit(parsed_script)
        self.name=visitor.name if visitor.name != "" else "Undefined"
        self.description=visitor.description if visitor.description != "" else "Undefined"
        self.test_steps=visitor.test_steps

    def generate_topology(self):
        with open(self.topology_dot, 'r') as dot_file:
            dot_graph = dot_file.read()
            graph = graphviz.Source(dot_graph)
            graph.render(self.topology_image, format='png', cleanup=True)

    def generate_specification(self):
        self.generate_topology()
        with open(self.specification, "w") as spec:
            spec.write(f"=== {self.name}\n")
            spec.write("==== Description\n")
            spec.write(self.description + "\n\n")
            spec.write("==== Topology\n")
            spec.write("ifdef::topdoc[]\n")
            spec.write(f"image::../../{self.test_dir}/topology.png[{self.name} topology]\n")
            spec.write("endif::topdoc[]\n")
            spec.write("ifndef::topdoc[]\n")
            spec.write("ifdef::testgroup[]\n")
            spec.write(f"image::{Path(*self.test_dir.parts[3:])}/topology.png[{self.name} topology]\n")
            spec.write("endif::testgroup[]\n")
            spec.write("ifndef::testgroup[]\n")
            spec.write(f"image::topology.png[{self.name} topology]\n")
            spec.write("endif::testgroup[]\n")
            spec.write("endif::topdoc[]\n")
            spec.write("==== Test sequence\n")
            spec.writelines([f". {step}\n" for step in self.test_steps])
            spec.write("\n\n<<<\n\n") # need empty lines to pagebreak

def parse_directory_tree(directory):
    directories=[]
    for dirpath, dirnames, filenames in os.walk(directory):
        testscript=False
        topology=False
        # Search for directories containing a test.py and a topology
        # and define the directory as a test directory

        if filenames:
            for filename in filenames:
                if filename == "test.py":
                    testscript=True
                if filename == "topology.dot":
                    topology=True
            if testscript and topology:
                directories.append(dirpath)
    return directories

parser = argparse.ArgumentParser(description="Generate a test specification for a subtree.")
parser.add_argument("-d", "--directory", required=True, help="The directory to parse.")
parser.add_argument("-r", "--root-dir", help="Path that all paths should be relative to")
args=parser.parse_args()

directories=parse_directory_tree(args.directory)
for directory in directories:
    test_case=TestCase(directory, args.root_dir)
    test_case.generate_specification()
