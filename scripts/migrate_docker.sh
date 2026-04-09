#!/bin/sh
set -e

if [ $# -gt 0 ]; then
    exec migrate -path /relayer/db/migrations "$@"
fi

urlencode_grouped_case () {
    string=$1; format=; set --
    while
        literal=${string%%[!-._~0-9A-Za-z]*}
        case "$literal" in
            ?*)
            format=$format%s
            set -- "$@" "$literal"
            string=${string#$literal};;
        esac
        case "$string" in
          "") false;;
        esac
    do
        tail=${string#?}
        head=${string%$tail}
        format=$format%%%02x
        set -- "$@" "'$head"
        string=$tail
    done
    printf "$format\\n" "$@"
}

export PGHOST=$(cat /relayer/config/config.yml | yq .postgres.hostname)
export PGPORT=$(cat /relayer/config/config.yml | yq .postgres.port)
export PGPASSWORD=$(aws rds generate-db-auth-token --hostname $PGHOST --port $PGPORT --username $PGUSER)
export PGPASSWORD=$(urlencode_grouped_case "$PGPASSWORD")

migrate -path /relayer/db/migrations -database "postgres://$PGUSER:$PGPASSWORD@$PGHOST:$PGPORT/relayer?sslmode=require" up
