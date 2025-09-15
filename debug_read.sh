#!/bin/bash

echo "=== Debug opx read issue ==="

echo "1. Testing direct op CLI:"
op read --account=YOPUYSOQIRHYVGIV3IQ5CS627Y "op://Private/AnthropicAPI/credential" && echo "✅ Direct op works"

echo ""
echo "2. Testing daemon status:"
OPX_AUTHD_PATH=./bin/opx-authd ./bin/opx status && echo "✅ Daemon responds"

echo ""
echo "3. Testing opx read (capturing both stdout and stderr separately):"
echo "STDOUT:"
OPX_AUTHD_PATH=./bin/opx-authd ./bin/opx read --account=YOPUYSOQIRHYVGIV3IQ5CS627Y "op://Private/AnthropicAPI/credential" 2>/tmp/opx_stderr
echo ""
echo "STDERR:"
cat /tmp/opx_stderr
rm -f /tmp/opx_stderr