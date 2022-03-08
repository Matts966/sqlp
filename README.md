# sqlp

## Features

- Load gcloud config automatically
- TUI with loading spinner
- Copy elements to clipboard
- Console editor integrations
- Support inputs from files, process substitution, and stdin

![gsod](demo/gsod.gif)

sqlp is a Preview pager for SQL (BigQuery).

## Installation

```sh
go install github.com/Matts966/sqlp@latest
```

## Usage

```sh
$ # with file path
$ sqlp file.sql
$ # with process substitution
$ sqlp <(echo "select * from \`bigquery-public-data.samples.gsod\`")
$ # with stdin
$ echo "select * from \`bigquery-public-data.samples.gsod\`" | sqlp
```

## Editor Integration

### Vim

```vim
" Use sqlp with current file
command! -nargs=0 Sqlp tabedit % | terminal sqlp %
```

![github_vim](demo/github_vim.gif)

### Emacs

Pull Requests are welcome!

## Supported databases

- [x] BigQuery
  - [ ] Support `CREATE TABLE AS SELECT` statements
- [ ] Presto
- [ ] Hive
- [ ] Snowflake
- [ ] PostgreSQL
- [ ] MySQL

## Loadmap

- [x] Add loading UI
- [ ] CI/CD
