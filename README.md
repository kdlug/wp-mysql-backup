# Wordpress mySql backup
A simple application to making mysql database backups. It parses wp-config.php file in order to get database credentials, dumps database to .sql file and optionally pushes it to git repository (tested with gitlab).

Project contains an example key pair (within ssh folder) for test purpose, but on production you should generate your own keys.

## Sample directory structure

```txt
|
|-- backups --> sql backup files
|-- data    --> mysql database files
|-- ssh     --> ssh keys
|-- vendor  --> go external libs
|-- www     --> wordpress project files
| Dockerfile
| docker-compose.yml
| main.go
| Makefile
| ...
```

## Build

Project uses `dep` package manager [https://github.com/golang/dep](). It can be installed via get:

```sh
go get -u github.com/golang/dep/cmd/dep
```

### Run locally

To run the app locally just type:

```sh
# install dependencies and build a binary 
make
# run with default configuration
./wp-mysql-backup
```

### Run in docker container

To run the app in a container type:

```sh
# create docker image
make pack
# run with default configuration
make run
```

## Usage
To create dump file just run:

```sh
./wp-mysql-backup [-outputDir=/backups] [-wpConfig=/var/www/wordpress/wp-config.php]
````

Parameters:
`-outputDir` - output directory to store database dump (default "/backups")
`-wpConfig` - path to wordpress configuration file (wp-config.php) (default "/var/www/wordpress/wp-config.php")

Each time a backup will be stored in given location (`outputDir`) under different filename (with datetime suffix) - `dump-[timestamp].sql`.

### Store backup in git repository

You have to create a git repository, f.ex. in gitlab.com, generate your ssh key pair and set a deploy key in gitlab with write access.

```sh
./wp-mysql-backup git \
-repositoryUrl="git@gitlab.com:username/repository-name.git" \
-privateKeyPath="~/.ssh/id_rsa" \
-authorName="John Doe" \
-authorEmail="john.doe@gmail.com"
````

Parameters:
`-authorEmail` - Git commit author email. (Required)
`-authorName` -  Git commit author name. (Required)
`-privateKeyPath` - Private key path for git login via ssh. (Required)
`-repositoryUrl` - Git repository url (f.ex. git@gitlab.com:username/repository-name.git). (Required)

## Todo

- [ ] Add cron
- [ ] Divide source code to separate packages
- [ ] Write tests
- [ ] Add environments
- [ ] Add possibility to provide extra parameters to `make run` (or via env)
- [ ] Minimize docker image