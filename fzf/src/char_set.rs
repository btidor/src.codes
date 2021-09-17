/// A bit vector representing a set of ASCII characters. Case insensitive.
///
/// [CharSet] tracks which characters are present in a string or collection of
/// strings. We use it to prune the directory tree by eliminating directories
/// whose paths don't contain all of the characters of the query. Internally,
/// [CharSet] uses a [u128] to store this information, allowing for fast
/// comparisons.
#[derive(Clone, Copy, Debug)]
pub struct CharSet {
    data: u128,
}

impl CharSet {
    /// Creates a new, empty character set.
    pub fn new() -> CharSet {
        CharSet { data: 0 }
    }

    /// Adds a character to the character set. Characters with a value of 128 or
    /// greater are out-of-range and are all mapped to a single bit (bit 0).
    pub fn add(&mut self, ch: char) {
        let mut lu = u32::from(ch.to_ascii_lowercase());
        if lu > 127 {
            lu = 0;
        }
        self.data |= 1 << lu;
    }

    /// Adds another [CharSet] to the character set. `other` is left unchanged,
    /// while `self` is updated to contain the union of both sets.
    pub fn incorporate(&mut self, other: &Self) {
        self.data |= other.data;
    }

    /// Returns true if and only if every character in `other` is also present
    /// in this character set.
    pub fn covers(&self, other: &Self) -> bool {
        self.data & other.data == other.data
    }

    /// Returns the internal [u128] representation of the character set. For
    /// tests and debugging.
    pub fn extract_internals(&self) -> u128 {
        self.data
    }
}

#[cfg(test)]
mod test {
    use super::*;

    #[test]
    fn new() {
        let cs = CharSet::new();
        assert_eq!(0x0, cs.extract_internals());
    }

    #[test]
    fn add() {
        let mut cs = CharSet::new();
        cs.add('a');
        assert_eq!(
            0x00000002_00000000_00000000_00000000,
            cs.extract_internals()
        );

        cs.add('B');
        assert_eq!(
            0x00000006_00000000_00000000_00000000,
            cs.extract_internals()
        );

        cs.add(' ');
        assert_eq!(
            0x00000006_00000000_00000001_00000000,
            cs.extract_internals()
        );

        cs.add('🦀');
        assert_eq!(
            0x00000006_00000000_00000001_00000001,
            cs.extract_internals()
        );

        cs.add('b');
        assert_eq!(
            0x00000006_00000000_00000001_00000001,
            cs.extract_internals()
        );

        cs.add('b');
        assert_eq!(
            0x00000006_00000000_00000001_00000001,
            cs.extract_internals()
        );
    }

    #[test]
    fn incorporate() {
        let mut cs0 = CharSet::new();
        cs0.add('A');

        let mut cs1 = CharSet::new();
        cs1.add(' ');

        cs0.incorporate(&cs1);
        assert_eq!(
            0x00000002_00000000_00000001_00000000,
            cs0.extract_internals()
        );
    }

    #[test]
    fn covers() {
        let mut cs0 = CharSet::new();
        cs0.add('A');
        cs0.add('B');

        let mut cs1 = CharSet::new();
        cs1.add('a');

        assert!(cs0.covers(&cs1));
        assert!(!cs1.covers(&cs0));
    }
}
