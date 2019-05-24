# Translator

Translates any word to specified language

This is simple wrapper over translate-shell and redis database


## Usage

See swagger.yaml

GET HTTP request to `/translate/:word/to/:lang`

response is a plain text


## Install

By [system-gen](https://github.com/reddec/system-gen)

* Download archive from [releases](https://github.com/lang-import/translate-backend/releases)
* Unpack on server
* Modify parameters in `system.json`
* Generate systemd files by `system-gen generate`
* Install by `./generated/install.sh`
* Start service by `systemctl start translate-backend` 
