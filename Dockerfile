FROM alpine:3.21

RUN apk add --no-cache ca-certificates
COPY esxport /usr/local/bin/esxport

EXPOSE 9272

ENTRYPOINT ["esxport"]
CMD ["serve"]
