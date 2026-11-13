const formatDate = (iso) => {
  const [year, month, day] = iso.split('-');
  return `${day}.${month}.${year}`;
};

export const userLine = (user) => `${user.name}, since ${formatDate(user.joined)}`;
