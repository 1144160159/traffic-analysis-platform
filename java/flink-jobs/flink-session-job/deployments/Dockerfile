FROM flink:1.18.1-java11

ARG JAR_FILE=target/flink-session-job-1.0.0-SNAPSHOT.jar
COPY ${JAR_FILE} /opt/flink/usrlib/flink-session-job.jar

RUN chown flink:flink /opt/flink/usrlib/flink-session-job.jar
USER flink

# Submit to the repository's Kubernetes Flink session cluster. The Kubernetes
# Job is a finite submitter; the remotely submitted Flink job owns the runtime.
ENTRYPOINT ["/opt/flink/bin/flink"]
CMD ["run", "-d", "-m", "flink-jobmanager.flink.svc:8081", "-c", "com.traffic.flink.session.SessionJob", "/opt/flink/usrlib/flink-session-job.jar"]
