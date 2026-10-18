use std::collections::HashMap;
use std::sync::Mutex;

/// What a [`TokenStore`] reports when it fails.
pub type StoreError = Box<dyn std::error::Error + Send + Sync>;

/// Where the session is kept between runs of the app — normally the operating system's credential
/// store (Windows Credential Manager, macOS Keychain, the Secret Service).
///
/// The shape is the one OS keychain libraries share: one secret per key, read, written and removed
/// whole. An adapter is a few lines:
///
/// ```ignore
/// struct Keychain(MyKeychain);
///
/// impl authway_native::TokenStore for Keychain {
///     fn get(&self, key: &str) -> Result<Option<String>, authway_native::StoreError> {
///         Ok(self.0.get(key)?)
///     }
///     fn set(&self, key: &str, value: &str) -> Result<(), authway_native::StoreError> {
///         Ok(self.0.set(key, value)?)
///     }
///     fn delete(&self, key: &str) -> Result<(), authway_native::StoreError> {
///         Ok(self.0.delete(key)?)
///     }
/// }
/// ```
///
/// The value holds the refresh token. Keep it where only the user can read it.
pub trait TokenStore: Send + Sync {
    /// The value stored under `key`, or `None` when there is none.
    fn get(&self, key: &str) -> Result<Option<String>, StoreError>;
    /// Stores `value` under `key`, replacing what was there.
    fn set(&self, key: &str, value: &str) -> Result<(), StoreError>;
    /// Removes the value under `key`. Removing one that is not there is not an error.
    fn delete(&self, key: &str) -> Result<(), StoreError>;
}

/// A store that forgets everything when the process ends — for tests, and for command-line tools
/// that sign in for one run.
#[derive(Debug, Default)]
pub struct MemoryStore {
    values: Mutex<HashMap<String, String>>,
}

impl TokenStore for MemoryStore {
    fn get(&self, key: &str) -> Result<Option<String>, StoreError> {
        Ok(self.values.lock().unwrap_or_else(|e| e.into_inner()).get(key).cloned())
    }

    fn set(&self, key: &str, value: &str) -> Result<(), StoreError> {
        self.values
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .insert(key.to_string(), value.to_string());
        Ok(())
    }

    fn delete(&self, key: &str) -> Result<(), StoreError> {
        self.values.lock().unwrap_or_else(|e| e.into_inner()).remove(key);
        Ok(())
    }
}

impl<T: TokenStore + ?Sized> TokenStore for std::sync::Arc<T> {
    fn get(&self, key: &str) -> Result<Option<String>, StoreError> {
        (**self).get(key)
    }
    fn set(&self, key: &str, value: &str) -> Result<(), StoreError> {
        (**self).set(key, value)
    }
    fn delete(&self, key: &str) -> Result<(), StoreError> {
        (**self).delete(key)
    }
}
