use crate::CharSet;
use core::slice::Iter;
use std::convert::TryInto;
use std::error::Error;

/// A directory in the filesystem tree.
///
/// [Directory] can be used to refer to a package root, or any subtree in a
/// package. We walk this structure to enumerate the package contents.
pub struct Directory<'a> {
    /// The directory name.
    pub name: PathComponent<'a>,
    /// The files in this directory.
    pub files: Vec<PathComponent<'a>>,
    /// The subdirectories of this directory.
    pub children: Vec<Directory<'a>>,
    /// A bit vector indicating which ASCII characters appear in the names of
    /// this directory, its files and children (recursively).
    pub char_set: CharSet,
}

/// A file or directory name.
///
/// [PathComponent] is pre-processed so it can be iterated over efficiently.
pub struct PathComponent<'a> {
    /// The pre-processed [PChar]s in the path component.
    data: Vec<PChar>,
    /// A [str] representing the non-normalized printable name of the path
    /// component.
    pub text: &'a str,
    /// A [CharSet] representing the characters in the path component.
    pub char_set: CharSet,
}

/// A character in a [PathComponent].
pub struct PChar {
    /// Represents the ASCII value of the character. Guaranteed to be less than
    /// 128; other values are mapped to zero.
    pub byte: u8,
    /// The point value of matching this character based on its case and its
    /// predecessor in the string. Reflects the start-of-path bonus, the
    /// after-separator bonus, and the camel-case bonus.
    pub bonus: u8,
}

impl PathComponent<'_> {
    /// Returns an iterator over [PChar] objects representing the individual
    /// pre-processed characters of the path component.
    pub fn iter(&self) -> Iter<'_, PChar> {
        self.data.iter()
    }

    /// Returns the number of characters in the path component.
    pub fn len(&self) -> usize {
        self.data.len()
    }

    /// Constructs a [PathComponent] from a string. If `initial` is set, the
    /// string is treated as the start of a path, otherwise a leading `/` is
    /// prepended.
    pub fn from(text: &str, initial: bool) -> PathComponent {
        let mut data;
        let mut char_set = CharSet::new();
        let mut bonus;
        if initial {
            // Start of path: following character gets an 8-point bonus.
            bonus = 8;
            data = Vec::with_capacity(text.len());
        } else {
            data = Vec::with_capacity(text.len() + 1);

            // Internal path component; add a forward slash as path separator
            data.push(PChar {
                byte: '/' as u8,
                // This is a departure from the original algorithm in one edge
                // case: we do *not* give the forward slash bonus points for
                // appearing after a previous slash or other separator.
                bonus: 0,
            });
            char_set.add('/');

            // The character following the slash gets a 5-point bonus.
            bonus = 5;
        }

        for c in text.chars() {
            if (c as u32) < 128 && (c as u32 > 0) {
                if bonus == 0 && c.is_ascii_uppercase() {
                    // Bonus for being an internal uppercase character (e.g. in
                    // "camelCaseString", "C" and "S" receive a bonus). This is
                    // mutually exclusive with the post-separator bonus.
                    bonus = 2;
                }
                data.push(PChar {
                    byte: c as u8,
                    bonus,
                });
            } else {
                // If the character is outside the range [1, 127], rewrite it to
                // a null byte, which will never be matched.
                data.push(PChar { byte: 0, bonus: 0 });
            }
            char_set.add(c);

            // If this character is a separator, compute the bonus for the
            // following character.
            bonus = match c {
                '/' | '\\' => 5,
                '_' | '-' | '.' | ' ' | '\'' | '"' | ':' => 4,
                _ => 0,
            };
        }

        PathComponent {
            data,
            text,
            char_set,
        }
    }
}

impl Directory<'_> {
    pub fn new<'a>(
        name: PathComponent<'a>,
        files: Vec<PathComponent<'a>>,
        children: Vec<Directory<'a>>,
    ) -> Directory<'a> {
        let mut char_set = CharSet::new();
        char_set.incorporate(&name.char_set);
        files.iter().for_each(|f| char_set.incorporate(&f.char_set));
        children
            .iter()
            .for_each(|c| char_set.incorporate(&c.char_set));

        Directory {
            name,
            files,
            children,
            char_set,
        }
    }

    /// Decodes a vector of [Directory] objects from a MessagePack byte slice.
    pub fn load(cur: &[u8]) -> Result<Vec<Directory>, Box<dyn Error + '_>> {
        let mut cur = cur;
        let n = rmp::decode::read_array_len(&mut cur)?;
        let mut dirs: Vec<Directory> = Vec::with_capacity(n.try_into().unwrap());
        for _ in 0..n {
            let directory;
            rmp::decode::read_bin_len(&mut cur).unwrap();
            (directory, cur) = Directory::decode(cur, true)?;
            dirs.push(directory);
        }

        Ok(dirs)
    }

    /// Decodes a single [Directory] and its contents, recursively, from a
    /// MessagePack byte slice. Returns the the [Directory] and a slice
    /// referring to the remaining contents of the byte slice.
    fn decode<'a>(
        cur: &'a [u8],
        initial: bool,
    ) -> Result<(Directory, &'a [u8]), Box<dyn Error + 'a>> {
        let mut cur = cur;
        rmp::decode::read_array_len(&mut cur).unwrap();

        let (nom, mut cur) = rmp::decode::read_str_from_slice(cur)?;
        let name = PathComponent::from(nom, initial);

        let n = rmp::decode::read_array_len(&mut cur).unwrap_or(0);
        let mut files: Vec<PathComponent> = Vec::with_capacity(n.try_into().unwrap());
        for _ in 0..n {
            let file;
            (file, cur) = rmp::decode::read_str_from_slice(cur)?;
            files.push(PathComponent::from(file, false));
        }

        let n = rmp::decode::read_array_len(&mut cur).unwrap_or(0);
        let mut children: Vec<Directory> = Vec::with_capacity(n.try_into().unwrap());
        for _ in 0..n {
            let child;
            (child, cur) = Directory::decode(cur, false)?;
            children.push(child);
        }

        Ok((Directory::new(name, files, children), cur))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn path_component_simple() {
        let pc = PathComponent::from("FooBarBaz.rs", true);
        assert_eq!("FooBarBaz.rs", pc.text);
        assert_eq!(12, pc.len());

        let chars: Vec<&PChar> = pc.iter().collect();
        assert_eq!(12, chars.len());
        assert_eq!(
            vec![70, 111, 111, 66, 97, 114, 66, 97, 122, 46, 114, 115],
            chars.iter().map(|x| x.byte).collect::<Vec<u8>>()
        );
        assert_eq!(
            vec![8, 0, 0, 2, 0, 0, 2, 0, 0, 0, 4, 0],
            chars.iter().map(|x| x.bonus).collect::<Vec<u8>>()
        );

        assert_eq!(
            0x040C8046_00000000_00004000_00000000,
            pc.char_set.extract_internals()
        );
    }

    #[test]
    fn path_component_complex() {
        let pc = PathComponent::from("a/b🦀:C", false);
        assert_eq!("a/b🦀:C", pc.text);
        assert_eq!(7, pc.len());

        let chars: Vec<&PChar> = pc.iter().collect();
        assert_eq!(7, chars.len());
        assert_eq!(
            vec![47, 97, 47, 98, 0, 58, 67],
            chars.iter().map(|x| x.byte).collect::<Vec<u8>>()
        );
        assert_eq!(
            vec![0, 5, 0, 5, 0, 0, 4],
            chars.iter().map(|x| x.bonus).collect::<Vec<u8>>()
        );

        assert_eq!(
            0x0000000E_00000000_04008000_00000001,
            pc.char_set.extract_internals()
        );
    }

    #[test]
    fn directory_decode() {
        let demo = [
            0x93, 0xA4, 0x72, 0x6F, 0x6F, 0x74, 0x93, 0xA3, 0x66, 0x6F, 0x6F, 0xA3, 0x62, 0x61,
            0x72, 0xA3, 0x62, 0x61, 0x7A, 0x92, 0x93, 0xA6, 0x63, 0x68, 0x69, 0x6C, 0x64, 0x31,
            0x92, 0xA2, 0x66, 0x31, 0xA2, 0x66, 0x32, 0x90, 0x93, 0xA6, 0x63, 0x68, 0x69, 0x6C,
            0x64, 0x32, 0x93, 0xA2, 0x66, 0x31, 0xA2, 0x66, 0x32, 0xA2, 0x66, 0x33, 0x90,
        ];
        let (directory, remainder) = Directory::decode(&demo, true).unwrap();
        assert_eq!(0, remainder.len());

        assert_eq!("root", directory.name.text);

        assert_eq!(3, directory.files.len());
        assert_eq!("baz", directory.files[2].text);

        assert_eq!(2, directory.children.len());
        assert_eq!("child2", directory.children[1].name.text);

        assert_eq!(3, directory.children[1].files.len());
        assert_eq!("f2", directory.children[1].files[1].text)
    }

    #[test]
    fn directory_load() {
        let demo = [
            0x92, 0xC4, 0x0C, 0x93, 0xA5, 0x72, 0x6F, 0x6F, 0x74, 0x31, 0x91, 0xA3, 0x66, 0x6F,
            0x6F, 0x90, 0xC4, 0x0C, 0x93, 0xA5, 0x72, 0x6F, 0x6F, 0x74, 0x32, 0x91, 0xA3, 0x66,
            0x6F, 0x6F, 0x90,
        ];
        let directories = Directory::load(&demo).unwrap();
        assert_eq!(2, directories.len());
        assert_eq!("root1", directories[0].name.text);
        assert_eq!(
            0x00148040_00000000_00028000_00000000,
            directories[0].char_set.extract_internals()
        );
        assert_eq!(
            0x00148040_00000000_00048000_00000000,
            directories[1].char_set.extract_internals()
        );
        assert_eq!("foo", directories[1].files[0].text);
    }
}
