use criterion::{Criterion, criterion_group, criterion_main};
use fzf::{Arena, Matcher, Query};
use std::collections::BinaryHeap;
use std::fs::File;
use std::io::Read;

fn benchmark(c: &mut Criterion) {
    // Load index
    let mut a = Arena::new();
    let mut file = File::open("paths.fzf").unwrap();
    let mut buf = Vec::new();
    file.read_to_end(&mut buf).unwrap();
    let nodes = a.load(&buf[..]).unwrap();
    let query = Query::new("abseilabsl.c").unwrap();

    let mut group = c.benchmark_group("fzf");
    group.sample_size(50);

    group.bench_function("walk", |b| {
        // Walk index
        let mut h = BinaryHeap::new();
        b.iter(|| {
            for (_, node) in nodes.iter().enumerate() {
                Matcher::new(&query, 100, &a).walk(node, "", &mut h, true);
            }
        });
    });
    group.finish();
}

criterion_group!(benches, benchmark);
criterion_main!(benches);
