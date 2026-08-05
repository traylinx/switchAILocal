return {
    name = "cortex-answer-schema",
    display_name = "Cortex Answer Schema Steering",
    version = "1.0.0",
    description = "Supplies the classification_code enum that the Cortex generic answer prompt omits but validation enforces. Fires only on requests that already carry 'classification_code', so other traffic is untouched."
}
