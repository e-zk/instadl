instadl
=======

![screenshot](scrot.png)

Bulk download Instagram posts.

(anti)Features
--------------

- Minimal interface.
- No JavaScript.
- No database. Works entirely off filesystem.
- No account/authentication required.
- Only works for "public" posts.
- Download multiple posts as `.zip`.

Dependencies
------------

Depends on [instaloader](https://github.com/instaloader/instaloader) Python
script for downloading posts. Ensure it's installed and included in `$PATH`.

Install
-------

	go install go.zakaria.org/instadl@latest

Running
-------

	usage:	instadl [-d directory] [-s path] [-l listen_addr]
	where:
		-d	local path to /static directory (defaults to ./static). this is where
			posts are saved.
		-s	local path to style.css (defaults to /style.css in directory
			specified by -d).
		-l	listen address (defaults to "0.0.0.0:8585").

