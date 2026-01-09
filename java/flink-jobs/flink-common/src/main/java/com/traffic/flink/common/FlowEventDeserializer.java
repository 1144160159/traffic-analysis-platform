// java/flink-jobs/flink-common/src/main/java/com/traffic/flink/common/ProtoDeserializer.java
package com.traffic.flink.common;

import com.traffic.proto.v1.FlowEvent;
import com.traffic.proto.v1.SessionEvent;
import org.apache.flink.api.common.serialization.DeserializationSchema;
import org.apache.flink.api.common.typeinfo.TypeInformation;

public class FlowEventDeserializer implements DeserializationSchema<FlowEvent> {
    
    @Override
    public FlowEvent deserialize(byte[] message) throws Exception {
        return FlowEvent.parseFrom(message);
    }
    
    @Override
    public boolean isEndOfStream(FlowEvent nextElement) {
        return false;
    }
    
    @Override
    public TypeInformation<FlowEvent> getProducedType() {
        return TypeInformation.of(FlowEvent.class);
    }
}