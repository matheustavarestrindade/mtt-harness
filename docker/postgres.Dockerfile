FROM postgres:16-alpine

ARG PGVECTOR_VERSION=0.8.1
ARG PGVECTOR_SHA256=a9094dfb85ccdde3cbb295f1086d4c71a20db1d26bf1d6c39f07a7d164033eb4

# Keep the same PostgreSQL major version and Alpine base as the existing volume.
# LLVM bitcode is optional; the extension itself uses the portable C build.
RUN apk add --no-cache --virtual .vector-build build-base curl \
    && curl -fsSL "https://codeload.github.com/pgvector/pgvector/tar.gz/refs/tags/v${PGVECTOR_VERSION}" -o /tmp/pgvector.tar.gz \
    && echo "${PGVECTOR_SHA256}  /tmp/pgvector.tar.gz" | sha256sum -c - \
    && tar -xzf /tmp/pgvector.tar.gz -C /tmp \
    && cd "/tmp/pgvector-${PGVECTOR_VERSION}" \
    && make OPTFLAGS="" with_llvm=no \
    && make with_llvm=no install \
    && cd / \
    && rm -rf /tmp/pgvector* \
    && apk del .vector-build
