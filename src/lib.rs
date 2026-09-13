use aes_gcm::{
    aead::{Aead, AeadCore, KeyInit, OsRng},
    Aes256Gcm, Nonce,
};
use anyhow::{anyhow, bail, Context, Result};
use chrono::{DateTime, NaiveDate, NaiveDateTime};
use regex::Regex;
use serde::Deserialize;
use serde_json::{Map, Number, Value};
use std::collections::HashMap;

#[derive(Debug, Clone, Deserialize)]
pub struct RuleStep {
    pub name: String,
    #[serde(default)]
    pub parameters: Map<String, Value>,
}

#[derive(Debug, Clone)]
pub struct MappingTargetField {
    pub id: String,
    pub field_name: String,
    pub data_type: String,
    pub encrypted: bool,
}

#[derive(Debug, Clone)]
pub struct MappingRule {
    pub source_id: String,
    pub target_field_id: String,
    pub source_field: String,
    pub transformations: Vec<RuleStep>,
    pub validations: Vec<RuleStep>,
}

#[derive(Debug, Clone, Default)]
pub struct RuleSet {
    pub sources: HashMap<String, String>,
    pub target_fields: HashMap<String, MappingTargetField>,
    pub rules: Vec<MappingRule>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PipelineError {
    pub failed_field: String,
    pub rule_name: String,
    pub error_message: String,
}

pub fn merge_payloads(payloads: impl IntoIterator<Item = Map<String, Value>>) -> Map<String, Value> {
    let mut record = Map::new();
    for payload in payloads {
        record.extend(payload);
    }
    record
}

pub fn encrypt(key: &[u8], plaintext: &[u8]) -> Result<(Vec<u8>, Vec<u8>)> {
    let cipher = cipher(key)?;
    let nonce = Aes256Gcm::generate_nonce(&mut OsRng);
    let ciphertext = cipher.encrypt(&nonce, plaintext).context("AES-GCM encryption failed")?;
    Ok((ciphertext, nonce.to_vec()))
}

pub fn decrypt(key: &[u8], nonce: &[u8], ciphertext: &[u8]) -> Result<Vec<u8>> {
    let cipher = cipher(key)?;
    if nonce.len() != 12 { bail!("invalid nonce size: expected 12, got {}", nonce.len()) }
    cipher.decrypt(Nonce::from_slice(nonce), ciphertext).context("AES-GCM decryption failed")
}

pub fn envelope_decrypt(kek: &[u8], wrapped_key: &[u8], payload_nonce: &[u8], payload: &[u8]) -> Result<Vec<u8>> {
    if wrapped_key.len() < 12 { bail!("wrapped DEK too short") }
    let (dek_nonce, encrypted_dek) = wrapped_key.split_at(12);
    let dek = decrypt(&normalized_key(kek), dek_nonce, encrypted_dek).context("failed to decrypt DEK")?;
    decrypt(&dek, payload_nonce, payload)
}

pub fn envelope_encrypt(kek: &[u8], wrapped_key: &[u8], plaintext: &[u8]) -> Result<(Vec<u8>, Vec<u8>)> {
    if wrapped_key.len() < 12 { bail!("wrapped DEK too short") }
    let (dek_nonce, encrypted_dek) = wrapped_key.split_at(12);
    let dek = decrypt(&normalized_key(kek), dek_nonce, encrypted_dek).context("failed to decrypt DEK")?;
    encrypt(&dek, plaintext)
}

pub fn generate_wrapped_dek(kek: &[u8]) -> Result<Vec<u8>> {
    let mut dek = [0_u8; 32];
    use aes_gcm::aead::rand_core::RngCore;
    OsRng.fill_bytes(&mut dek);
    let (encrypted_dek, nonce) = encrypt(&normalized_key(kek), &dek)?;
    let mut wrapped = nonce;
    wrapped.extend(encrypted_dek);
    Ok(wrapped)
}

pub fn process_payload(
    payload: &Map<String, Value>,
    source_id: &str,
    rules: &RuleSet,
    master_key: &[u8],
    wrapped_key: &[u8],
) -> (Map<String, Value>, Vec<PipelineError>) {
    let mut result = Map::new();
    let mut errors = Vec::new();
    for rule in rules.rules.iter().filter(|rule| rule.source_id == source_id) {
        let Some(target) = rules.target_fields.get(&rule.target_field_id) else { continue };
        let input = payload.iter().find(|(name, _)| name.eq_ignore_ascii_case(&rule.source_field))
            .map(|(_, value)| value.clone()).unwrap_or(Value::Null);
        let value = match apply_transformations(input, &rule.transformations) {
            Ok(value) => value,
            Err(error) => { errors.push(pipeline_error(rule, "transformation", error)); continue; }
        };
        if let Err(error) = apply_validations(&value, &rule.validations) {
            errors.push(pipeline_error(rule, "validation", error));
            continue;
        }
        let value = match cast_to_schema(value, &target.data_type) {
            Ok(value) => value,
            Err(error) => { errors.push(pipeline_error(rule, "auto_cast", error)); continue; }
        };
        let value = if target.encrypted && !value.is_null() && !is_empty(&value) {
            match envelope_encrypt(master_key, wrapped_key, value_to_string(&value).as_bytes()) {
                Ok((ciphertext, nonce)) => serde_json::json!({"ciphertext": ciphertext, "nonce": nonce}),
                Err(error) => { errors.push(pipeline_error(rule, "encryption", error)); continue; }
            }
        } else { value };
        result.insert(target.field_name.clone(), value);
    }
    (result, errors)
}

fn pipeline_error(rule: &MappingRule, rule_name: &str, error: anyhow::Error) -> PipelineError {
    PipelineError { failed_field: rule.source_field.clone(), rule_name: rule_name.into(), error_message: error.to_string() }
}

fn apply_transformations(mut value: Value, chain: &[RuleStep]) -> Result<Value> {
    for step in chain.iter().filter(|step| !step.name.is_empty()) {
        value = transform(&step.name, value, &step.parameters)
            .with_context(|| format!("step {} failed", step.name))?;
    }
    Ok(value)
}

fn apply_validations(value: &Value, chain: &[RuleStep]) -> Result<()> {
    for step in chain.iter().filter(|step| !step.name.is_empty()) {
        validate(&step.name, value, &step.parameters)
            .with_context(|| format!("step {} failed", step.name))?;
    }
    Ok(())
}

fn transform(name: &str, value: Value, params: &Map<String, Value>) -> Result<Value> {
    match name {
        "trim_whitespace" => Ok(string_value(&value).map(|s| Value::String(s.trim().into())).unwrap_or(value)),
        "to_upper" => Ok(string_value(&value).map(|s| Value::String(s.to_uppercase())).unwrap_or(value)),
        "to_lower" => Ok(string_value(&value).map(|s| Value::String(s.to_lowercase())).unwrap_or(value)),
        "default_value" => if value.is_null() || is_empty(&value) {
            params.get("value").cloned().ok_or_else(|| anyhow!("missing 'value' parameter for default_value"))
        } else { Ok(value) },
        "regex_replace" => {
            let Some(text) = string_value(&value) else { return Ok(value) };
            if text.is_empty() { return Ok(Value::String(text)) }
            let pattern = parameter_string(params, "pattern")?;
            let replacement = parameter_string(params, "replace")?;
            Ok(Value::String(Regex::new(pattern).context("invalid regex pattern")?.replace_all(&text, replacement).into_owned()))
        }
        "parse_date" => parse_date(value, params),
        "string_split" => {
            let Some(text) = string_value(&value) else { return Ok(value) };
            if text.is_empty() { return Ok(Value::String(text)) }
            let separator = parameter_string(params, "separator")?;
            let index = parameter_usize(params, "index")?;
            text.split(separator).nth(index).map(|part| Value::String(part.into()))
                .ok_or_else(|| anyhow!("index {index} out of bounds for split result"))
        }
        "cast_type" => cast_value(value, parameter_string(params, "target_type")?),
        _ => bail!("transform function '{name}' not found in registry"),
    }
}

fn validate(name: &str, value: &Value, params: &Map<String, Value>) -> Result<()> {
    match name {
        "not_null" => {
            if value.is_null() { bail!("value is null") }
            if is_empty(value) { bail!("value is an empty string") }
            Ok(())
        }
        "regex_match" => {
            if value.is_null() || is_empty(value) { return Ok(()) }
            let re = Regex::new(parameter_string(params, "pattern")?).context("invalid regex pattern")?;
            if re.is_match(&value_to_string(value)) { Ok(()) } else { bail!("value does not match pattern") }
        }
        "email" => {
            if value.is_null() || is_empty(value) { return Ok(()) }
            let re = Regex::new(r"^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$").expect("valid static email regex");
            if re.is_match(&value_to_string(value)) { Ok(()) } else { bail!("value is not a valid email address") }
        }
        "in_list" => {
            if value.is_null() || is_empty(value) { return Ok(()) }
            let allowed = params.get("allowed").and_then(Value::as_array).ok_or_else(|| anyhow!("'allowed' parameter must be a list"))?;
            if allowed.iter().any(|item| value_to_string(item) == value_to_string(value)) { Ok(()) } else { bail!("value is not in the allowed list") }
        }
        "range_check" => {
            if value.is_null() || is_empty(value) { return Ok(()) }
            let number = value.as_f64().ok_or_else(|| anyhow!("value is not numeric"))?;
            if let Some(min) = params.get("min").and_then(Value::as_f64) { if number < min { bail!("value is less than minimum {min}") } }
            if let Some(max) = params.get("max").and_then(Value::as_f64) { if number > max { bail!("value is greater than maximum {max}") } }
            Ok(())
        }
        "min_length" => validate_length(value, params, true),
        "max_length" => validate_length(value, params, false),
        _ => bail!("validate function '{name}' not found in registry"),
    }
}

fn validate_length(value: &Value, params: &Map<String, Value>, minimum: bool) -> Result<()> {
    if value.is_null() || is_empty(value) { return Ok(()) }
    let length = parameter_usize(params, "length")?;
    let actual = value_to_string(value).len();
    if (minimum && actual < length) || (!minimum && actual > length) { bail!("value length ({actual}) is outside permitted bound ({length})") }
    Ok(())
}

fn parse_date(value: Value, params: &Map<String, Value>) -> Result<Value> {
    let Some(text) = string_value(&value) else { return Ok(value) };
    if text.is_empty() { return Ok(Value::String(text)) }
    let input = params.get("input_format").and_then(Value::as_str);
    let date = if let Some(format) = input { parse_with_format(&text, format)? } else { autodetect_date(&text)? };
    let output = params.get("output_format").and_then(Value::as_str).unwrap_or("2006-01-02");
    Ok(Value::String(format_date(date, output)?))
}

fn autodetect_date(text: &str) -> Result<NaiveDate> {
    for format in ["%Y-%m-%d", "%Y/%m/%d", "%d.%m.%Y", "%d.%m.%y", "%m/%d/%Y", "%d/%m/%Y", "%d-%m-%Y", "%m-%d-%Y"] {
        if let Ok(date) = NaiveDate::parse_from_str(text, format) { return Ok(date) }
    }
    if let Ok(datetime) = DateTime::parse_from_rfc3339(text) { return Ok(datetime.date_naive()) }
    for format in ["%Y-%m-%dT%H:%M:%S", "%Y-%m-%d %H:%M:%S"] {
        if let Ok(datetime) = NaiveDateTime::parse_from_str(text, format) { return Ok(datetime.date()) }
    }
    bail!("unsupported date format: {text}")
}

fn parse_with_format(text: &str, layout: &str) -> Result<NaiveDate> {
    let format = go_layout(layout)?;
    NaiveDate::parse_from_str(text, format).or_else(|_| NaiveDateTime::parse_from_str(text, format).map(|date| date.date()))
        .with_context(|| format!("failed to parse date using {layout}"))
}

fn format_date(date: NaiveDate, format: &str) -> Result<String> {
    if format == "2006-01-02T15:04:05Z07:00" || format == time_rfc3339() {
        return Ok(date.and_hms_opt(0, 0, 0).unwrap().and_utc().to_rfc3339_opts(chrono::SecondsFormat::Secs, true));
    }
    Ok(date.format(go_layout(format)?).to_string())
}

fn time_rfc3339() -> &'static str { "2006-01-02T15:04:05Z07:00" }

fn go_layout(format: &str) -> Result<&str> {
    match format {
        "2006-01-02" => Ok("%Y-%m-%d"), "02.01.2006" => Ok("%d.%m.%Y"),
        "01/02/2006" => Ok("%m/%d/%Y"), "02/01/2006" => Ok("%d/%m/%Y"),
        "02-01-2006" => Ok("%d-%m-%Y"), "01-02-2006" => Ok("%m-%d-%Y"),
        "2006/01/02" => Ok("%Y/%m/%d"), "02.01.06" => Ok("%d.%m.%y"),
        _ => bail!("unsupported Go date layout: {format}"),
    }
}

fn cast_to_schema(value: Value, target_type: &str) -> Result<Value> {
    if value.is_null() || target_type.is_empty() { return Ok(value) }
    cast_value(value, target_type)
}

fn cast_value(value: Value, target_type: &str) -> Result<Value> {
    let text = value_to_string(&value);
    match target_type.to_ascii_lowercase().as_str() {
        "int" | "integer" | "int64" => Ok(Value::Number(Number::from(text.parse::<i64>().context("failed to cast to integer")?))),
        "float" | "float64" | "numeric" | "decimal" => Number::from_f64(text.parse::<f64>().context("failed to cast to float")?)
            .map(Value::Number).ok_or_else(|| anyhow!("float is not a finite JSON number")),
        "bool" | "boolean" => Ok(Value::Bool(text.parse::<bool>().context("failed to cast to bool")?)),
        "string" | "text" | "varchar" => Ok(Value::String(text)),
        _ => bail!("unsupported target_type: {target_type}"),
    }
}

fn normalized_key(key: &[u8]) -> Vec<u8> { key.iter().copied().chain(std::iter::repeat(0)).take(32).collect() }
fn cipher(key: &[u8]) -> Result<Aes256Gcm> { Aes256Gcm::new_from_slice(key).map_err(|_| anyhow!("AES-256 key must be exactly 32 bytes")) }
fn string_value(value: &Value) -> Option<String> { value.as_str().map(str::to_owned) }
fn is_empty(value: &Value) -> bool { value.as_str().is_some_and(str::is_empty) }
fn value_to_string(value: &Value) -> String { match value { Value::String(value) => value.clone(), Value::Null => "null".into(), _ => value.to_string() } }
fn parameter_string<'a>(params: &'a Map<String, Value>, name: &str) -> Result<&'a str> { params.get(name).and_then(Value::as_str).ok_or_else(|| anyhow!("missing or invalid '{name}' parameter")) }
fn parameter_usize(params: &Map<String, Value>, name: &str) -> Result<usize> { params.get(name).and_then(Value::as_u64).map(|value| value as usize).ok_or_else(|| anyhow!("missing or invalid '{name}' parameter")) }

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn envelope_round_trip() {
        let kek = [7; 32];
        let wrapped = generate_wrapped_dek(&kek).unwrap();
        let (ciphertext, nonce) = envelope_encrypt(&kek, &wrapped, b"secret").unwrap();
        assert_eq!(envelope_decrypt(&kek, &wrapped, &nonce, &ciphertext).unwrap(), b"secret");
    }

    #[test]
    fn transforms_and_validations_match_contract() {
        let params = serde_json::json!({"pattern": "[^0-9]", "replace": ""}).as_object().unwrap().clone();
        assert_eq!(transform("regex_replace", Value::String("ab12".into()), &params).unwrap(), Value::String("12".into()));
        let params = serde_json::json!({"allowed": ["active"]}).as_object().unwrap().clone();
        assert!(validate("in_list", &Value::String("active".into()), &params).is_ok());
    }

    #[test]
    fn pipeline_encrypts_transformed_values() {
        let mut rules = RuleSet::default();
        rules.target_fields.insert("target".into(), MappingTargetField {
            id: "target".into(), field_name: "email".into(), data_type: "text".into(), encrypted: true,
        });
        rules.rules.push(MappingRule {
            source_id: "source".into(), target_field_id: "target".into(), source_field: "Mail".into(),
            transformations: vec![RuleStep { name: "to_lower".into(), parameters: Map::new() }],
            validations: vec![RuleStep { name: "email".into(), parameters: Map::new() }],
        });
        let kek = [9; 32];
        let wrapped = generate_wrapped_dek(&kek).unwrap();
        let (result, errors) = process_payload(
            &serde_json::json!({"mail": "TEST@EXAMPLE.COM"}).as_object().unwrap().clone(),
            "source", &rules, &kek, &wrapped,
        );
        assert!(errors.is_empty());
        assert!(result["email"]["ciphertext"].is_array());
    }
}
