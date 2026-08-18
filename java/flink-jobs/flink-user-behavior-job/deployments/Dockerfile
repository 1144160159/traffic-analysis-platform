FROM flink:1.18.1-java11
ARG JAR_FILE=target/flink-user-behavior-job-1.0.0-SNAPSHOT.jar
COPY ${JAR_FILE} /opt/flink/usrlib/flink-user-behavior-job.jar
USER flink
ENTRYPOINT ["/opt/flink/bin/flink"]
CMD ["run", "-d", "-m", "flink-jobmanager.flink.svc:8081", "/opt/flink/usrlib/flink-user-behavior-job.jar"]
