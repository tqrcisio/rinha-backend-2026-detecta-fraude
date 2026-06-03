FROM gcc:13 AS build
COPY lb/lb.c /lb.c
RUN gcc -O2 -static -o /lb /lb.c

FROM scratch
COPY --from=build /lb /lb
ENTRYPOINT ["/lb"]
