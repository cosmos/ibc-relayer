FROM migrate/migrate
RUN apk add --no-cache yq aws-cli

WORKDIR /relayer

COPY ./db/migrations /relayer/db/migrations

COPY ./scripts/migrate_docker.sh /relayer/migrate.sh
RUN chmod +x /relayer/migrate.sh

ENTRYPOINT [ "/relayer/migrate.sh" ]
