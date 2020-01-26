#!/usr/bin/env python
import base64
import os
import os.path

# Get the file directory
fdir = os.path.dirname(os.path.realpath(__file__))

# Get the shell scripts
shell_scripts = [f for f in os.listdir(fdir) if f.endswith(".sh")]

# Read each and dump as base64
for sh in shell_scripts:
    raw = open(os.path.join(fdir,sh)).read()
    enc = base64.b64encode(raw)
    print "Base64 for %s:" % sh
    print enc
    print

