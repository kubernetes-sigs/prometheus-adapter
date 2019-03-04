FROM BASEIMAGE
RUN mkdir -vp /etc/ssl
COPY adapter /
COPY cacert.pem /etc/ssl/ca-bundle.pem
USER 1001:1001
ENTRYPOINT ["/adapter"]
