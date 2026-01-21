use crate::CharSet;
use std::convert::TryInto;
use std::error::Error;
use std::io::Read;

/// A directory in the filesystem tree.
///
/// [Directory] can be used to refer to a package root, or any subtree in a
/// package. We walk this structure to enumerate the package contents.
pub struct Directory {
    /// The directory name.
    pub name: PathComponent,
    /// The files & subdirectories in this directory.
    pub fileoff: u32,
    pub diroff: u32,
    pub filelen: u16,
    pub dirlen: u16,
    /// A bit vector indicating which ASCII characters appear in the names of
    /// this directory, its files and children (recursively).
    pub char_set: CharSet,
}

/// A file or directory name.
///
/// [PathComponent] is pre-processed so it can be iterated over efficiently.
#[derive(Clone, Copy, Debug)]
pub struct PathComponent {
    /// The pre-processed [PChar]s in the path component.
    data: u32, // slice into arena.pchars: 24 bits offset, 8 bits length
    /// A [CharSet] representing the characters in the path component.
    pub char_set: CharSet,
}

impl PathComponent {
    pub fn len(self) -> usize {
        usize::try_from(self.data & 0xFF).unwrap()
    }
}

/// A character in a [PathComponent].
#[derive(Clone, Copy, Debug)]
pub struct PChar {
    /// Represents the ASCII value of the character. Guaranteed to be less than
    /// 128; other values are mapped to zero.
    pub byte: u8,
    /// The point value of matching this character based on its case and its
    /// predecessor in the string. Reflects the start-of-path bonus, the
    /// after-separator bonus, and the camel-case bonus.
    pub bonus: u8,
}

pub struct Arena {
    pub pchars: Vec<PChar>,
    pub files: Vec<PathComponent>,
    pub dirs: Vec<Directory>,
}

impl Arena {
    pub fn new() -> Arena {
        return Arena {
            pchars: vec![],
            files: vec![],
            dirs: vec![],
        };
    }

    /// Constructs a [PathComponent] from a string. If `initial` is set, the
    /// string is treated as the start of a path, otherwise a leading `/` is
    /// prepended.
    pub fn path_component(&mut self, text: &str) -> PathComponent {
        while self.pchars.len() % 4 != 0 {
            // Align to 4 bytes. 9.6% overhead.
            self.pchars.push(PChar { byte: 0, bonus: 0 });
        }
        let start = self.pchars.len();
        let mut len: usize = 0;

        let mut char_set = CharSet::new();
        char_set.add('/');

        // Give a 5-point bonus to any character that begins a path component.
        // This is a slight departure from the original algorithm, which gives
        // an additional +3 to the first character in the path and also gives
        // points to the slash character when it is repeated or follows another
        // separator.
        let mut bonus = 5;

        for c in text.chars() {
            if (c as u32) < 128 && (c as u32 > 0) {
                if bonus == 0 && c.is_ascii_uppercase() {
                    // Bonus for being an internal uppercase character (e.g. in
                    // "camelCaseString", "C" and "S" receive a bonus). This is
                    // mutually exclusive with the post-separator bonus.
                    bonus = 2;
                }
                self.pchars.push(PChar {
                    byte: c as u8,
                    bonus,
                });
                len += 1
            } else {
                // If the character is outside the range [1, 127], rewrite it to
                // a null byte, which will never be matched.
                self.pchars.push(PChar { byte: 0, bonus: 0 });
                len += 1
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

        if (start >= (1 << 26)) || (start & 0x3 != 0) {
            panic!("arena filled past max pchars") // needs to fit in 24 bits
        }
        let n = u8::try_from(len).expect("path component too long");
        let data = u32::try_from(start << 6 | usize::from(n)).unwrap();
        PathComponent { data, char_set }
    }

    /// Returns an iterator over [PChar] objects representing the individual
    /// pre-processed characters of the path component.
    pub fn path_iter(&self, pc: PathComponent) -> &[PChar] {
        let start = usize::try_from(pc.data >> 6).unwrap();
        let len = usize::try_from(pc.data & 0xFF).unwrap();
        return &self.pchars[start..(start + len)];
    }

    pub fn path_text(&self, pc: PathComponent) -> String {
        let mut text = String::with_capacity(usize::from(pc.len()));
        for pc in self.path_iter(pc) {
            text.push(pc.byte.into())
        }
        return text;
    }

    /// Decodes a single [Directory] and its contents, recursively, from a
    /// MessagePack byte slice. Returns the the [Directory] and a slice
    /// referring to the remaining contents of the byte slice.
    fn parse_directory(&mut self, io: &mut impl Read) -> Result<Directory, Box<dyn Error>> {
        rmp::decode::read_array_len(io).unwrap();

        let n = rmp::decode::read_str_len(io)?;
        let mut buf = vec![0u8; n as usize];
        io.read_exact(&mut buf)?;
        let name = self.path_component(std::str::from_utf8(&buf)?);

        let fileoff = u32::try_from(self.files.len()).expect("arena filled past max files");
        let filelen = u16::try_from(rmp::decode::read_array_len(io).unwrap_or(0))
            .expect("too many files in directory");
        for _ in 0..filelen {
            let n = rmp::decode::read_str_len(io)?;
            let mut buf = vec![0u8; n as usize];
            io.read_exact(&mut buf)?;
            let file = self.path_component(std::str::from_utf8(&buf)?);
            self.files.push(file);
        }

        let diroff = u32::try_from(self.dirs.len()).expect("arena filled past max dirs");
        let dirlen = u16::try_from(rmp::decode::read_array_len(io).unwrap_or(0))
            .expect("too many subdirs in directory");
        for _ in 0..dirlen {
            self.dirs.push(Directory {
                name,
                fileoff,
                diroff,
                filelen,
                dirlen,
                char_set: CharSet::new(),
            });
        }
        for i in diroff..(diroff + u32::from(dirlen)) {
            self.dirs[usize::try_from(i).unwrap()] = self.parse_directory(io)?;
        }

        Ok(self.directory(name, fileoff, diroff, filelen, dirlen))
    }

    pub fn directory(
        &self,
        name: PathComponent,
        fileoff: u32,
        diroff: u32,
        filelen: u16,
        dirlen: u16,
    ) -> Directory {
        let mut char_set = CharSet::new();
        char_set.incorporate(&name.char_set);
        _iter(&self.files, fileoff, filelen)
            .iter()
            .for_each(|f| char_set.incorporate(&f.char_set));
        _iter(&self.dirs, diroff, dirlen)
            .iter()
            .for_each(|c| char_set.incorporate(&c.char_set));

        Directory {
            name,
            fileoff,
            diroff,
            filelen,
            dirlen,
            char_set,
        }
    }

    /// Decodes a vector of [Directory] objects from a MessagePack byte slice.
    pub fn load(&mut self, io: &mut impl Read) -> Result<Vec<Directory>, Box<dyn Error>> {
        let n = rmp::decode::read_array_len(io)?;
        let mut dirs: Vec<Directory> = Vec::with_capacity(n.try_into().unwrap());
        for _ in 0..n {
            rmp::decode::read_bin_len(io).unwrap();
            let directory = self.parse_directory(&mut *io)?;
            dirs.push(directory);
        }
        self.pchars.shrink_to_fit();

        Ok(dirs)
    }

    pub fn files_iter(&self, d: &Directory) -> &[PathComponent] {
        _iter(&self.files, d.fileoff, d.filelen)
    }

    pub fn dirs_iter(&self, d: &Directory) -> &[Directory] {
        _iter(&self.dirs, d.diroff, d.dirlen)
    }
}

fn _iter<T>(v: &Vec<T>, off: u32, len: u16) -> &[T] {
    let start = usize::try_from(off).unwrap();
    let len = usize::from(len);
    &v[start..(start + len)]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn path_component_simple() {
        let mut a = Arena::new();
        let pc = a.path_component("FooBarBaz.rs");
        assert_eq!("FooBarBaz.rs", a.path_text(pc));
        assert_eq!(12, pc.len());

        let chars: Vec<PChar> = a.path_iter(pc).to_vec();
        assert_eq!(12, chars.len());
        assert_eq!(
            vec![70, 111, 111, 66, 97, 114, 66, 97, 122, 46, 114, 115],
            chars.iter().map(|x| x.byte).collect::<Vec<u8>>()
        );
        assert_eq!(
            vec![5, 0, 0, 2, 0, 0, 2, 0, 0, 0, 4, 0],
            chars.iter().map(|x| x.bonus).collect::<Vec<u8>>()
        );

        assert_eq!(0x00000081_9008C00C, pc.char_set.extract_internals());
    }

    #[test]
    fn path_component_complex() {
        let mut a = Arena::new();
        let pc = a.path_component("a/b🦀:C");
        assert_eq!("a/b\0:C", a.path_text(pc));
        assert_eq!(6, pc.len());

        let chars: Vec<PChar> = a.path_iter(pc).to_vec();
        assert_eq!(6, chars.len());
        assert_eq!(
            vec![97, 47, 98, 0, 58, 67],
            chars.iter().map(|x| x.byte).collect::<Vec<u8>>()
        );
        assert_eq!(
            vec![5, 0, 5, 0, 0, 4],
            chars.iter().map(|x| x.bonus).collect::<Vec<u8>>()
        );

        assert_eq!(0x00000000_0001C009, pc.char_set.extract_internals());
    }

    #[test]
    fn directory_decode() {
        let mut a = Arena::new();
        let mut demo: &[u8] = &[
            0x93, 0xA4, 0x72, 0x6F, 0x6F, 0x74, 0x93, 0xA3, 0x66, 0x6F, 0x6F, 0xA3, 0x62, 0x61,
            0x72, 0xA3, 0x62, 0x61, 0x7A, 0x92, 0x93, 0xA6, 0x63, 0x68, 0x69, 0x6C, 0x64, 0x31,
            0x92, 0xA2, 0x66, 0x31, 0xA2, 0x66, 0x32, 0x90, 0x93, 0xA6, 0x63, 0x68, 0x69, 0x6C,
            0x64, 0x32, 0x93, 0xA2, 0x66, 0x31, 0xA2, 0x66, 0x32, 0xA2, 0x66, 0x33, 0x90,
        ];
        let directory = a.parse_directory(&mut demo).unwrap();
        assert_eq!("root", a.path_text(directory.name));

        assert_eq!(3, directory.filelen);
        assert_eq!("baz", a.path_text(a.files_iter(&directory)[2]));

        assert_eq!(2, directory.dirlen);
        assert_eq!("child2", a.path_text(a.dirs_iter(&directory)[1].name));

        assert_eq!(3, a.dirs_iter(&directory)[1].filelen);
        assert_eq!(
            "f2",
            a.path_text(a.files_iter(&a.dirs_iter(&directory)[1])[1])
        );
    }

    #[test]
    fn directory_load() {
        let mut a = Arena::new();
        let mut demo: &[u8] = &[
            0x92, 0xC4, 0x0C, 0x93, 0xA5, 0x72, 0x6F, 0x6F, 0x74, 0x31, 0x91, 0xA3, 0x66, 0x6F,
            0x6F, 0x90, 0xC4, 0x0C, 0x93, 0xA5, 0x72, 0x6F, 0x6F, 0x74, 0x32, 0x91, 0xA3, 0x66,
            0x6F, 0x6F, 0x90,
        ];
        let directories = a.load(&mut demo).unwrap();
        assert_eq!(2, directories.len());
        assert_eq!("root1", a.path_text(directories[0].name));
        assert_eq!(
            0x00000002_90080028,
            directories[0].char_set.extract_internals()
        );
        assert_eq!(
            0x00000002_90080048,
            directories[1].char_set.extract_internals()
        );
        assert_eq!("foo", a.path_text(a.files_iter(&directories[1])[0]));
    }
}
