import 'package:flutter_test/flutter_test.dart';
import 'package:lapin_app/features/annotations/domain/quote_anchor.dart';

void main() {
  group('locateQuote', () {
    const String source = '第一段讲工具。中间一段。第二段也讲工具。';

    test('唯一命中时返回它的起点', () {
      expect(locateQuote(source, '中间一段', 0), source.indexOf('中间一段'));
    });

    test('多处命中时取离提示最近的一处', () {
      final int first = source.indexOf('讲工具');
      final int second = source.lastIndexOf('讲工具');
      expect(locateQuote(source, '讲工具', 0), first);
      expect(locateQuote(source, '讲工具', second + 3), second);
      // 提示落在两者中间偏后，应当选后一处
      expect(locateQuote(source, '讲工具', (first + second) ~/ 2 + 2), second);
    });

    test('提示正好落在一处命中上时直接采用，不再扫描', () {
      final int second = source.lastIndexOf('讲工具');
      expect(locateQuote(source, '讲工具', second), second);
    });

    test('偏移按 UTF-16 计数，与服务端一致', () {
      // 服务端用 len(utf16.Encode(...)) 校验，Dart 的字符串下标本身就是 UTF-16。
      const String withEmoji = '开头🙂中间标记';
      expect(locateQuote(withEmoji, '标记', 0), withEmoji.indexOf('标记'));
      expect('开头🙂中间'.length, 6, reason: '🙂 占两个 UTF-16 码元');
    });

    test('找不到或引用为空时返回 null', () {
      expect(locateQuote(source, '不存在的话', 0), isNull);
      expect(locateQuote(source, '', 0), isNull);
    });
  });

  group('matchSelectionInSource', () {
    test('精确命中时原样返回', () {
      const String source = '这是一段普通的正文。';
      final QuoteMatch? match = matchSelectionInSource(source, '一段普通');
      expect(match, isNotNull);
      expect(match!.text, '一段普通');
      expect(match.start, source.indexOf('一段普通'));
      expect(match.end, match.start + '一段普通'.length);
    });

    test('跨软换行的选区能还原成源里的字面子串', () {
      // 源里是硬换行，渲染时被折成空格，所以选出来的文字与源不同。
      const String source = 'The quick brown\nfox jumps.';
      final QuoteMatch? match = matchSelectionInSource(source, 'brown fox');
      expect(match, isNotNull, reason: '放宽空白后应当命中');
      expect(match!.text, 'brown\nfox', reason: '存进服务端的必须是源里的那一段');
      expect(source.contains(match.text), isTrue, reason: '服务端会做 Contains 校验');
    });

    test('选区两端的空白会被去掉', () {
      const String source = '前面 中间 后面';
      final QuoteMatch? match = matchSelectionInSource(source, '  中间  ');
      expect(match!.text, '中间');
    });

    test('给出所在块时，在块内定位，避免锚到别处的同名文字', () {
      const String source = '第一段提到工具。\n\n第二段也提到工具，这里才是选中的地方。';
      final QuoteMatch? match = matchSelectionInSource(
        source,
        '工具',
        blockText: '第二段也提到工具，这里才是选中的地方。',
      );
      expect(match!.start, source.lastIndexOf('工具'));
    });

    test('块文本自身跨软换行时也能用来缩小范围', () {
      const String source = '第一段提到工具。\n\n第二段也提到工具，\n这里才是选中的地方。';
      final QuoteMatch? match = matchSelectionInSource(
        source,
        '工具',
        blockText: '第二段也提到工具， 这里才是选中的地方。',
      );
      expect(match!.start, source.lastIndexOf('工具'));
    });

    test('还原不出来时返回 null，不做猜测', () {
      const String source = '正文里没有这句话。';
      expect(matchSelectionInSource(source, '凭空出现的引用'), isNull);
      expect(matchSelectionInSource(source, '   '), isNull);
    });

    test('选区含正则元字符时按字面处理', () {
      const String source = r'调用 fn(a.b) 之后返回。';
      final QuoteMatch? match = matchSelectionInSource(source, 'fn(a.b)');
      expect(match!.text, 'fn(a.b)');
      expect(match.start, source.indexOf('fn(a.b)'));
    });
  });
}
