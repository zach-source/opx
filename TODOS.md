* Change `opx login` to have 2 sub commands `opx login vault` and `ops login 1password`, let the second variable (vault, 1password) be dependent on the backend so we can have a pluggable backend archecture
* Under logins, allow a setup that will can use another command (maybe opx) to provide the password to itself in cases where authentication is required frequnently
* Provide support for brew installs
* provide support for nix flake installs
* provide support for nix home module
