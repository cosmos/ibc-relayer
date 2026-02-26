FROM migrate/migrate

COPY ./db/migrations /migrations

ENTRYPOINT ["migrate", "-path", "/migrations"]
