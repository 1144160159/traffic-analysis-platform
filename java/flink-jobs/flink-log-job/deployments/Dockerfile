FROM flink:1.18-scala_2.12

ARG JAR_FILE=target/flink-log-job-1.0.0-SNAPSHOT.jar

COPY ${JAR_FILE} /opt/flink/usrlib/flink-log-job.jar

RUN chown -R flink:flink /opt/flink/usrlib

USER flink

ENTRYPOINT ["/opt/flink/bin/flink"]
CMD ["run", "-Dstate.checkpoints.dir=file:///home/k8s-data/flink/checkpoints/log-job", "-Dstate.savepoints.dir=file:///home/k8s-data/flink/savepoints/log-job", "-d", "-m", "flink-jobmanager.flink.svc:8081", "/opt/flink/usrlib/flink-log-job.jar"]
