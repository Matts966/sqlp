# sqlp

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
$ sqlp <(echo "SELECT * FROM table")
$ # with stdin
$ echo "SELECT * FROM table" | sqlp
```

## Supported databases

- [x] BigQuery
- [ ] Snowflake
- [ ] PostgreSQL
- [ ] MySQL
