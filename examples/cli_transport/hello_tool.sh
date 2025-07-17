#!/usr/bin/env bash
# hello_tool.sh - Executes the say_hello tool

# Read the tool invocation from stdin
input=$(cat)

# For this simple example, just return a greeting
echo '{
  "result": "Hello! This is a friendly greeting from the CLI tool."
}'